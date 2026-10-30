package main

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	width  = 2560
	height = 1600
)

type rgb struct{ r, g, b float64 }

type palette struct {
	values map[string]rgb
	light  bool
}

var entry = regexp.MustCompile(`^([a-z_0-9]+) *= *"(#[0-9a-fA-F]{6}|[a-z]+)"`)

func parse(hex string) (rgb, bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return rgb{}, false
	}
	value, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{float64(value >> 16 & 255), float64(value >> 8 & 255), float64(value & 255)}, true
}

func load(path string) (palette, error) {
	file, err := os.Open(path)
	if err != nil {
		return palette{}, err
	}
	defer file.Close()
	result := palette{values: map[string]rgb{}}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := entry.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if match == nil {
			continue
		}
		if match[1] == "mode" {
			result.light = match[2] == "light"
			continue
		}
		if value, ok := parse(match[2]); ok {
			result.values[match[1]] = value
		}
	}
	return result, scanner.Err()
}

func (p palette) get(names ...string) rgb {
	for _, name := range names {
		if value, ok := p.values[name]; ok {
			return value
		}
	}
	return rgb{128, 128, 128}
}

func mix(a, b rgb, t float64) rgb {
	t = math.Max(0, math.Min(1, t))
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func smooth(edge0, edge1, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-edge0)/(edge1-edge0)))
	return t * t * (3 - 2*t)
}

type noise struct{ seed uint32 }

func (n noise) lattice(x, y int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + n.seed*2147483647
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h) / float64(math.MaxUint32)
}

func (n noise) value(x, y float64) float64 {
	fx, fy := math.Floor(x), math.Floor(y)
	ix, iy := int(fx), int(fy)
	tx, ty := smooth(0, 1, x-fx), smooth(0, 1, y-fy)
	top := n.lattice(ix, iy) + (n.lattice(ix+1, iy)-n.lattice(ix, iy))*tx
	bottom := n.lattice(ix, iy+1) + (n.lattice(ix+1, iy+1)-n.lattice(ix, iy+1))*tx
	return top + (bottom-top)*ty
}

func (n noise) fbm(x, y float64, octaves int) float64 {
	total, amplitude, norm := 0.0, 0.5, 0.0
	for range octaves {
		total += n.value(x, y) * amplitude
		norm += amplitude
		x, y = x*2.03+17.1, y*2.03+9.2
		amplitude *= 0.5
	}
	return total / norm
}

func seed(text string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(text))
	return h.Sum32()
}

func render(shade func(x, y float64) rgb, grain noise) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var group sync.WaitGroup
	rows := make(chan int, height)
	for y := range height {
		rows <- y
	}
	close(rows)
	for range runtime.NumCPU() {
		group.Add(1)
		go func() {
			defer group.Done()
			for y := range rows {
				for x := range width {
					c := shade(float64(x)/width, float64(y)/height)
					d := (grain.lattice(x, y) - 0.5) * 2.4
					img.SetRGBA(x, y, color.RGBA{clamp(c.r + d), clamp(c.g + d), clamp(c.b + d), 255})
				}
			}
		}()
	}
	group.Wait()
	return img
}

func clamp(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

func depths(p palette) (rgb, rgb) {
	base := p.get("background")
	if p.light {
		return base, mix(base, p.get("foreground"), 0.30)
	}
	return base, p.get("darker_background", "dark_background", "background")
}

func ridges(p palette, n noise) func(x, y float64) rgb {
	base, deep := depths(p)
	accent := p.get("accent", "blue")
	glow := p.get("magenta", "accent")
	sunX, sunY := 0.68+n.value(3.1, 7.7)*0.16, 0.26+n.value(9.3, 1.2)*0.12
	const layers = 6
	return func(x, y float64) rgb {
		top := deep
		if p.light {
			top = base
		}
		sky := mix(mix(top, accent, 0.10), mix(base, glow, 0.24), smooth(0.05, 0.72, y))
		dx, dy := (x-sunX)*float64(width)/float64(height), y-sunY
		distance := math.Hypot(dx, dy)
		sky = mix(sky, mix(accent, glow, 0.25), (1-smooth(0.100, 0.104, distance))*0.92)
		sky = mix(sky, accent, (1-smooth(0.10, 0.42, distance))*0.14)
		result := sky
		for layer := range layers {
			t := float64(layer) / float64(layers-1)
			horizon := 0.50 + t*0.40
			amplitude := 0.20 - t*0.09
			frequency := 1.5 + t*1.7
			line := horizon + (n.fbm(x*frequency+float64(layer)*11.3, float64(layer)*3.7, 5)-0.5)*amplitude*2
			if y >= line {
				far := mix(base, accent, 0.40)
				tone := mix(far, deep, math.Pow(t, 0.8))
				result = mix(tone, mix(tone, deep, 0.35), smooth(line, line+0.32, y))
			}
		}
		return result
	}
}

func contours(p palette, n noise) func(x, y float64) rgb {
	base, deep := depths(p)
	accent := p.get("accent", "blue")
	second := p.get("cyan", "green", "accent")
	aspect := float64(width) / float64(height)
	return func(x, y float64) rgb {
		wash := mix(base, deep, smooth(0.0, 1.0, (x+y)/2))
		height := func(px, py float64) float64 {
			return n.fbm(px*aspect*2.1, py*2.1, 4)*9 + px*0.8
		}
		const step = 0.0008
		field := height(x, y)
		slope := math.Hypot(height(x+step, y)-field, height(x, y+step)-field) / step
		nearest := math.Round(field)
		distance := math.Abs(field-nearest) / math.Max(slope, 0.5)
		major := math.Mod(nearest, 5) == 0
		thickness := 0.00042
		strength := 0.30
		if major {
			thickness, strength = 0.00080, 0.62
		}
		line := 1 - smooth(thickness*0.55, thickness, distance)
		tint := mix(accent, second, n.value(x*2.5, y*2.5))
		return mix(wash, tint, line*strength)
	}
}

func waves(p palette, n noise) func(x, y float64) rgb {
	base, deep := depths(p)
	tones := []rgb{p.get("accent", "blue"), p.get("blue", "accent"), p.get("cyan", "green"), p.get("magenta", "accent"), p.get("accent", "blue"), p.get("green", "cyan"), p.get("blue", "accent")}
	offset := n.value(1.5, 2.5) * 6.28
	return func(x, y float64) rgb {
		result := mix(mix(deep, tones[0], 0.08), base, smooth(0, 1, y))
		for index, tone := range tones {
			t := float64(index) / float64(len(tones)-1)
			center := 0.34 + t*0.60
			swell := math.Sin(x*(2.6+t*1.3)*math.Pi+offset+t*5.1) * (0.075 - t*0.03)
			swell += (n.fbm(x*1.7+t*7, t*3.3, 3) - 0.5) * 0.16
			line := center + swell
			if y >= line {
				depth := smooth(line, line+0.42, y)
				result = mix(result, mix(tone, base, 0.32+depth*0.5), 0.34-t*0.10)
			}
			result = mix(result, tone, (1-smooth(0, 0.0016, math.Abs(y-line)))*0.42)
		}
		return result
	}
}

func save(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return jpeg.Encode(file, img, &jpeg.Options{Quality: 92})
}

func preview(path string, img *image.RGBA, p palette) error {
	const w, h = 1280, 800
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			out.Set(x, y, img.At(x*width/w, y*height/h))
		}
	}
	names := []string{"background", "foreground", "red", "green", "yellow", "blue", "magenta", "cyan", "accent"}
	chip := w / len(names)
	for index, name := range names {
		c := p.get(name, "accent")
		for y := h - 36; y < h; y++ {
			for x := index * chip; x < (index+1)*chip; x++ {
				out.SetRGBA(x, y, color.RGBA{clamp(c.r), clamp(c.g), clamp(c.b), 255})
			}
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, out)
}

func generate(dir string) error {
	p, err := load(filepath.Join(dir, "colors.toml"))
	if err != nil {
		return err
	}
	name := filepath.Base(dir)
	backgrounds := filepath.Join(dir, "backgrounds")
	if err := os.MkdirAll(backgrounds, 0o755); err != nil {
		return err
	}
	grain := noise{seed(name + "grain")}
	styles := []struct {
		file  string
		shade func(x, y float64) rgb
	}{
		{"1-ridges.jpg", ridges(p, noise{seed(name + "ridges")})},
		{"2-contours.jpg", contours(p, noise{seed(name + "contours")})},
		{"3-waves.jpg", waves(p, noise{seed(name + "waves")})},
	}
	for index, style := range styles {
		img := render(style.shade, grain)
		if err := save(filepath.Join(backgrounds, style.file), img); err != nil {
			return err
		}
		if index == 0 {
			if err := preview(filepath.Join(dir, "preview.png"), img, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	dirs, err := filepath.Glob(filepath.Join("themes", "*"))
	if err != nil || len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "run it from the repository root, with the themes in themes/")
		os.Exit(1)
	}
	if len(os.Args) > 1 {
		dirs = os.Args[1:]
	}
	for _, dir := range dirs {
		if err := generate(dir); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			os.Exit(1)
		}
		fmt.Println(dir)
	}
}

# hyprtrixie themes

The themes for [hyprtrixie](https://github.com/Ahmadlolbit/hyprtrixie). Each theme is a folder in `themes/`:

| File | What it is |
| --- | --- |
| `colors.toml` | the palette every app is coloured from |
| `icons.theme` | the icon theme to use |
| `backgrounds/` | wallpapers |
| `preview.png` | what the theme picker shows |
| `neovim.lua`, `vscode.json`, `btop.theme`, `chromium.theme`, `hyprland.lua` | settings for those apps, where a theme has them |

## Wallpapers

Every wallpaper here is generated from its theme's own colours by `tools/wallpapers`, so they match the theme and are free of anyone else's rights:

```bash
go run ./tools/wallpapers
```

## Adding a theme

Anything added here has to be safe to publish:

1. Its licence must allow copying, changing and republishing. "No licence" means no permission, however public it is.
2. Nothing that shows a character, a show, a game or a brand, whatever the repository it came from says.
3. Record where it came from and its licence in the pull request.

## License

GPL-3.0. Some theme files come from MIT-licensed work; see [THIRD-PARTY-LICENSES](THIRD-PARTY-LICENSES).

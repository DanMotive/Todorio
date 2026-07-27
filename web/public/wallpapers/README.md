# System wallpapers

Drop image files in this folder and list them in `wallpapers.json` to offer them in
Settings -> Appearance -> Wallpaper. Everything under `web/public` is bundled into the Go
binary with the rest of the frontend, so no server configuration is involved.

```json
[
  { "id": "mountains", "name": "Mountains", "file": "mountains.jpg", "dim": 0.5 }
]
```

| Field  | Required | Meaning                                                                 |
| ------ | -------- | ----------------------------------------------------------------------- |
| `file` | yes      | File name inside this folder. Served as `/wallpapers/<file>`.             |
| `id`   | no       | Stable key stored in the browser. Defaults to `sys:<file>`.               |
| `name` | no       | Label shown on hover. Defaults to the file name.                          |
| `dim`  | no       | Starting dim for this picture, `0`-`1`. Defaults to `0.55`.               |

Notes:

- Keep the `id` stable. Renaming it drops the choice of anyone already using that wallpaper
  back to no wallpaper, because the id is what the browser remembers.
- These files are downloaded by every client, so resize before committing. Roughly 1920px wide
  and a few hundred kilobytes of WebP or JPEG is plenty; the layer is blurred and dimmed anyway.
- Prefer calm, low-contrast pictures. Panels are translucent in rich mode, and busy detail
  behind them costs readability.
- A broken or missing manifest is not an error: the built-in gradients still work.

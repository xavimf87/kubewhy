# Brand assets

| File | What it is | Where it is used |
| --- | --- | --- |
| `logo.svg` | The mark: 723 bytes, no fonts, no external references. | README header, and anywhere the project needs an icon. |
| `banner.svg` | The mark with the wordmark and tagline, on a dark background. | Source for the social preview. |
| `social-preview.png` | `banner.svg` rendered at 1280×640. | Upload under **Settings → General → Social preview**. GitHub only accepts raster images there. |

## The mark

A heptagon, because that is the shape the Kubernetes ecosystem speaks in, with a question mark inside it, because the whole product is one question. The dot of the question mark is amber rather than blue: it is the finding — the one thing you were looking for.

It is drawn as paths rather than text, so it renders identically everywhere without depending on a font being installed. The gradient runs blue to cyan, and both ends stay legible on white and on GitHub's dark background, so a single file serves both themes. It stays readable down to 24 pixels.

## Regenerating the raster

The SVGs are the source. The PNG is derived and can be rebuilt with any renderer; this one was produced with headless Chrome, which is the same engine most people will view the README in:

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless --disable-gpu \
  --screenshot=docs/assets/social-preview.png \
  --window-size=1280,640 \
  "file://$PWD/docs/assets/banner.svg"
```

On Linux, `rsvg-convert -w 1280 -h 640 docs/assets/banner.svg -o docs/assets/social-preview.png` does the same job.

## Colours

| Role | Value |
| --- | --- |
| Ring and question mark | `#4A8BF5` → `#22D3EE` |
| The finding | `#F59E0B` |
| Banner background | `#0D1117` (GitHub's dark canvas) |

These are the terminal palette's own colours: the same blue that marks a command, the same amber that marks a warning.

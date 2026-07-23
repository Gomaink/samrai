# samrai v0.2.0-rc.9

Release candidate 9 fixes PDF tap navigation on physical iPhones.

## Mobile Safari PDF controls

The PDF reader now uses native, passive touch listeners for page navigation. This avoids relying on the Pointer Events behavior reproduced by desktop device emulation and works across the rendered canvas and selectable text layer in Mobile Safari.

A short, single-finger tap keeps the same layout:

- left third: previous page;
- center third: show or hide the controls;
- right third: next page.

The gesture is discarded when the finger moves, the page scrolls, another finger is added, a double tap begins, a long press is detected, or text selection becomes active. Links, buttons, inputs, annotation marks, highlights, area selection, pinch zoom, panning, and browser zoom remain untouched.

The reader also stops applying its temporary pointer-selection mode to touch input and now clears that mode after pointer or touch cancellation. This prevents an interrupted gesture from leaving the PDF text layer stuck.

## Docker Compose

The included Compose file pulls:

```text
ghcr.io/gomaink/samrai:0.2.0-rc.9
```

Host port `24600` remains mapped to container port `8080`.

## Upgrade from rc.8

1. Stop rc.8.
2. Extract rc.9 to a separate directory.
3. Copy the complete rc.8 `data` directory into rc.9.
4. Copy the rc.8 `.env` file into rc.9.
5. Run `test-windows.bat` or `go test ./...`.
6. Start rc.9 and verify a PDF on a physical iPhone.

No database migration is required. Books, progress, users, annotations, OCR data, and settings remain compatible.

# samrai v0.2.0-rc.8

Release candidate 8 fixes touch navigation in the PDF reader on phones and tablets.

## PDF touch navigation

The whole PDF page now responds to short taps, whether the finger lands on the rendered canvas or the selectable text layer:

- left third: previous page;
- center third: show or hide the reader controls;
- right third: next page.

The reader only treats a clean, single-finger tap as navigation. It leaves movement, scrolling, panning, pinch zoom, double-tap zoom, long presses, and text selection alone. Links, buttons, inputs, annotation marks, area selection, and note controls keep their normal behavior.

## Docker Compose

The included Compose file now uses the published image directly:

```text
ghcr.io/gomaink/samrai:0.2.0-rc.8
```

It maps host port `24600` to port `8080` in the container. Existing installations keep the same data volume behavior, including compatibility with the legacy `pageturner-data` volume name.

## Upgrade from rc.7

1. Stop rc.7.
2. Extract rc.8 to a separate directory.
3. Copy the complete rc.7 `data` directory into rc.8.
4. Copy the rc.7 `.env` file into rc.8.
5. Run `test-windows.bat` or `go test ./...`.
6. Start rc.8 and test a PDF on a touch device.

No database migration is required. Existing books, progress, users, annotations, OCR data, and settings remain compatible.

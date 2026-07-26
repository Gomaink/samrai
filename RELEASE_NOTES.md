# samrai v0.2.0-rc.10

Release candidate 10 restores EPUB tap navigation on physical iPhones.

## Mobile Safari EPUB controls

EPUB chapters are rendered inside a sandboxed iframe. The previous reader listened for a generated `click` inside that frame, which works in desktop device emulation but is not reliable in Mobile Safari. The reader now listens to native touch events in the EPUB document itself.

A short, single-finger tap uses the established layout and follows the publication reading direction:

- left third: previous page for left-to-right books;
- center third: show or hide the controls;
- right third: next page for left-to-right books.

The gesture is ignored after movement, scrolling, a second finger, a double tap, a long press, or active text selection. Links, audio, video, highlights, notes, and other interactive content keep their normal behavior. A handled touch also suppresses its delayed compatibility click, preventing duplicate navigation on iOS.

The PDF reader is unchanged in this release.

## Docker Compose

The included Compose file pulls:

```text
ghcr.io/gomaink/samrai:0.2.0-rc.10
```

Host port `24600` remains mapped to container port `8080`.

## Upgrade from rc.9

1. Stop rc.9.
2. Extract rc.10 to a separate directory.
3. Copy the complete rc.9 `data` directory into rc.10.
4. Copy the rc.9 `.env` file into rc.10.
5. Run `test-windows.bat` or wait for the repository CI checks.
6. Start rc.10 and test an EPUB on a physical iPhone.

No database migration is required. Books, progress, users, annotations, OCR data, and settings remain compatible.

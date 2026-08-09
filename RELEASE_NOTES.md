# samrai v0.2.0-rc.11

Release candidate 11 fixes EPUB tap navigation on physical iPhones by addressing the WebKit sandbox behavior that differs from desktop device emulation.

## Mobile Safari EPUB input

EPUB chapters are rendered in a same-origin sandboxed iframe. RC.10 installed native touch listeners in the chapter document, but Mobile Safari can block handlers installed by the parent page when the sandbox does not include script permission. Desktop Chromium device emulation does not reproduce that restriction, which is why the same reader appeared correct in responsive mode.

The EPUB frame now includes `allow-scripts` so parent-installed input handlers can run in WebKit. This does not enable JavaScript shipped by EPUB files: the EPUB document response still sends a Content Security Policy with `script-src 'none'`, and samrai removes script elements and inline event attributes from the loaded document. A regression test now checks that the CSP continues to block publication scripts.

Touch listeners are attached to the EPUB document element, and the outer iframe registers a passive touch listener as an additional WebKit routing safeguard. The existing gesture rules remain unchanged: a short single-finger tap navigates through the left and right zones or toggles controls in the center, while scrolling, multi-touch, double taps, long presses, text selection, links, and media are ignored by page navigation.

The EPUB content revision is bumped so cached chapter documents are revalidated after the upgrade. The PDF reader is unchanged.

## Docker Compose

The included Compose file pulls:

```text
ghcr.io/gomaink/samrai:0.2.0-rc.11
```

Host port `24600` remains mapped to container port `8080`.

## Upgrade from rc.10

1. Stop rc.10.
2. Extract rc.11 to a separate directory.
3. Copy the complete rc.10 `data` directory into rc.11.
4. Copy the rc.10 `.env` file into rc.11.
5. Run `test-windows.bat` or wait for the repository CI checks.
6. Start rc.11 and test an EPUB on a physical iPhone.

No database migration is required. Books, progress, users, annotations, OCR data, and settings remain compatible.

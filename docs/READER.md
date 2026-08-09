# Paged reader

## Navigation

The reader divides the viewport into left, center, and right interaction areas. The side areas change pages; the center toggles controls. Keyboard and button navigation call the same page-change logic.

Western and manga directions reverse the meaning of the side controls. The setting can come from book metadata and can be changed by the reader.

## Progress

Progress belongs to the signed-in user. The client stores page or EPUB position through the progress endpoint and records reading-session activity while the book is open.

Resetting progress does not delete favorites, history, or annotations.

## Image pages

Comic pages and fixed-layout pages use contain or width-oriented fitting according to the selected reader mode. Nearby pages are preloaded without starting an unbounded number of requests.

The page endpoint can return resized WebP images. Cached variants are disposable and are not included in backups.

## PDF reader

The browser uses PDF.js for rendering and text selection. The server stores catalog metadata, OCR policy, searchable text, annotations, and area rectangles. Passwords supplied for protected PDFs remain in the active browser session and are not saved in the database.

On touch screens, a short tap in the left third moves to the previous page, a tap in the center toggles the controls, and a tap in the right third moves to the next page. The reader listens for native touch events in capture mode so the gesture works over both the canvas and the selectable text layer, including Mobile Safari. The listeners are passive and do not cancel browser behavior. Movement, scrolling, panning, pinch zoom, double-tap zoom, long presses, text selection, annotation controls, links, buttons, and form fields keep their normal behavior.

## Reflowable EPUB reader

Reflowable books are rendered inside a same-origin sandboxed iframe. The reader exposes theme, font, width, alignment, line spacing, margins, and pagination controls. Position is stored as a structured locator rather than a visual page number.

On touch screens, the reader installs its tap handlers on the EPUB document rather than relying on a generated mouse click. Mobile Safari requires script permission on the sandbox before handlers installed by the parent page can run inside the frame, so the iframe carries `allow-scripts`. Publication scripts are still disabled by the EPUB response Content Security Policy (`script-src 'none'`) and removed from the loaded document. The parent frame also registers a passive touch listener and the inner handlers attach to the document element to avoid WebKit iframe touch-routing edge cases.

A short tap uses the left, center, and right reader zones. Movement, scrolling, multi-touch, double taps, long presses, text selection, links, media, and interactive elements do not trigger page navigation.

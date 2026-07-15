# samrai v0.2.0-rc.7

Release candidate 7 adds manual and bulk reading-status controls.

## Reading status

A book can now be marked as read or unread from its detail page. Changing the status does not move the reader or discard the saved page, EPUB section, or location.

The books view also supports selecting multiple titles and applying either status in one operation. Series pages include the same selection controls, plus a shortcut for marking the entire series as read or unread.

A book that has never been opened can still be marked as read. Marking it as unread later keeps the progress record at its initial position; use **Reset progress** when the book should return to **Not started**.

## Other fixes

- Manually completed books display 100% progress throughout the interface.
- The remaining Portuguese sort and filter labels were replaced with English text.
- Reading-status updates are atomic: a batch containing a missing book is rejected without changing the other books.

## Upgrade from rc.6

1. Stop rc.6.
2. Extract rc.7 to a new directory.
3. Copy the complete rc.6 `data` directory.
4. Copy the rc.6 `.env` file.
5. Run `test-windows.bat`.
6. Start rc.7.

No database migration is required. Existing progress, annotations, history, and library files remain compatible.

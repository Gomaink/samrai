# samrai v0.2.0-rc.6

Release candidate 6 fixes two test fixtures introduced while translating the repository to English.

## What was wrong

The application code was behaving correctly, but two PDF catalog tests no longer represented their original cases:

- the metadata limit test expected the word `title` after truncating a longer English phrase to six runes;
- the excerpt test searched for `montreal` and then required the original accented text `Montréal` to contain the unaccented spelling.

## Fix

The metadata test now uses `résumé` to verify trimming and Unicode rune boundaries. The excerpt test now uses an unaccented matched term, while accent-insensitive search remains covered by its dedicated test.

No database migration or runtime behavior changes are included in this release.

## Upgrade from rc.5

1. Stop rc.5.
2. Extract rc.6 to a new directory.
3. Copy the complete rc.5 `data` directory.
4. Copy the rc.5 `.env` file.
5. Run `test-windows.bat`.
6. Start rc.6.

All library data and settings remain compatible.

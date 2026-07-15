# Changelog

This file records user-visible changes. Dates use UTC.

## [0.2.0-rc.7] - 2026-07-15

### Added

- Added manual **Mark as read** and **Mark as unread** actions without discarding the saved page or EPUB location.
- Added bulk reading-status actions to the books view and series detail pages.
- Added a one-click action to mark an entire series as read or unread.

### Fixed

- Completed books now show a full 100% progress bar even when they were marked as read before reaching the final page.
- Replaced two remaining Portuguese filter labels in the library interface.

## [0.2.0-rc.6] - 2026-07-14

### Fixed

- Repaired two PDF catalog tests whose English fixtures no longer matched the behavior they were intended to verify.
- Kept the metadata limit test focused on Unicode rune boundaries and the excerpt test focused on matched-position trimming.

## [0.2.0-rc.5] - 2026-07-14

### Changed

- Made English the language of the web interface, API messages, logs, command output, scripts, tests, and documentation.
- Rewrote the repository documentation around reproducible setup and maintenance tasks.
- Changed the default managed library name to `Main library` without modifying custom names or user metadata.

### Added

- MIT license, contribution guide, conduct policy, issue forms, pull request template, EditorConfig, Git attributes, and Dependabot configuration.
- Environment-variable reference in `docs/CONFIGURATION.md`.
- Migration 015 for the untouched Portuguese default library label.

### Fixed

- Removed mixed Portuguese and English labels left in the library and favorites screens.
- Standardized PDF, EPUB, and combined notebook export filenames on the `-annotations.md` suffix.

## [0.2.0-rc.4] - 2026-07-14

### Added

- Batch import by multi-file selection or folder selection.
- Natural file ordering and filename-based series and volume suggestions.
- Per-file metadata review, progress, retry, and a two-upload concurrency limit.
- SHA-256 duplicate rejection before a new import job is created.

### Changed

- Increased the recent import-job window so large batches remain visible.
- Serialized quick PDF preparation after upload to avoid bursts of expensive work.

## [0.2.0-rc.3] - 2026-07-14

### Fixed

- Corrected the annotation query aliases used by the catalog and full notebook export.
- Added regression coverage for title search and exports containing more than one page of annotations.

## [0.2.0-rc.2] - 2026-07-14

### Changed

- Renamed the project from PageTurner to samrai.
- Reworked the visual identity around JetBrains Mono, compact panels, restrained violet accents, and an editorial terminal-inspired layout.

### Fixed

- Exported every annotation instead of only the first annotation for each book.
- Removed the 500-item limit from full notebook exports.
- Prevented a reading session key from being reassigned to another book.
- Kept prerelease images from replacing the stable Docker `latest` tag.
- Standardized source archive names and checksum manifests.

### Compatibility

- Kept support for old environment variables, database and log filenames, cookies, backups, and Docker volumes.

## [0.2.0-preview.6] - 2026-07-13

- Added favorites for books and series.
- Added reading-session history and a combined PDF/EPUB annotation notebook.
- Improved mobile page fitting and full-bleed reader presentation.

## [0.2.0-preview.5] - 2026-07-13

- Added fixed-layout and reflowable EPUB readers.
- Added EPUB search, highlights, notes, and reading progress.
- Added backup, restore, system information, logs, audit records, and support bundles.

## [0.2.0-preview.4] - 2026-07-13

- Added CBR, CB7, and CBT imports with prepared page storage.
- Added archive-tool detection and stricter archive validation.

## [0.2.0-preview.3] - 2026-07-12

- Added PDF classification, selective OCR, search, password handling, highlights, notes, and area annotations.
- Added per-document PDF policy controls.

## [0.2.0-preview.2] - 2026-07-12

- Added the React library interface, series views, metadata editing, page caching, and responsive readers.

## [0.2.0-preview.1] - 2026-07-12

- Added persistent import jobs, managed storage, migration support, and the first browser upload flow.

## [0.1.0] - 2026-07-12

- Initial Go server with SQLite, setup, authentication, sessions, health checks, and the embedded web application.

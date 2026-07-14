# samrai

samrai is a self-hosted library and reader for manga, comics, PDFs, and EPUB books. It runs as a single Go server with the web application embedded in the binary and stores its state in SQLite.

> **Release status:** `v0.2.0-rc.6` is a release candidate. Back up the `data` directory before upgrading and report anything that blocks normal reading or library management.

<p align="center">
  <a href="https://imgur.com/FkzDTBn">
    <img src="https://i.imgur.com/FkzDTBn.png" alt="samrai library interface" width="1100">
  </a>
</p>

## What it does

- Imports CBZ, CBR, CB7, CBT, PDF, and DRM-free EPUB files.
- Accepts multiple files or a whole folder in one batch.
- Extracts common series and volume patterns from filenames before upload.
- Groups books into series and keeps per-user reading progress.
- Provides paged readers for comics, PDFs, fixed-layout EPUBs, and reflowable EPUBs.
- Supports PDF search, selective OCR, highlights, notes, and area annotations.
- Supports EPUB search, highlights, notes, and table-of-contents navigation.
- Keeps favorites, reading sessions, and a combined annotation notebook private to each user.
- Includes administrator accounts, reader accounts, backups, audit records, logs, and diagnostics.
- Generates resized WebP pages on demand and keeps them in a rebuildable disk cache.

## Supported formats

| Format | Import | Reader | Notes |
| --- | :---: | :---: | --- |
| CBZ / ZIP | Yes | Paged images | Read directly after archive validation |
| CBR / RAR | Yes | Paged images | Requires 7-Zip or `lsar` + `unar` |
| CB7 / 7z | Yes | Paged images | Requires 7-Zip or `lsar` + `unar` |
| CBT / TAR | Yes | Paged images | Handled without external tools |
| PDF | Yes | Native paged reader | Search, OCR, highlights, notes, area marks |
| EPUB 2 / 3 | Yes | Fixed or reflowable | DRM and encrypted content are not supported |

## Requirements

For a native installation:

- Go 1.26 or newer;
- Tesseract OCR, optional, for scanned PDFs and damaged text layers;
- 7-Zip or `lsar` + `unar`, optional, for CBR and CB7 imports.

Node.js 22 is only needed when changing the frontend. Release source archives already contain a compiled web build.

Docker includes the OCR and archive tools used by samrai.

## Quick start on Windows

Open PowerShell in the extracted project directory:

```powershell
Copy-Item .env.example .env

go version
go mod tidy
.\test-windows.bat
.\run-windows.bat
```

Open `http://127.0.0.1:8080/`. The first visit asks you to create the administrator account.

The Windows scripts load `.env`, keep Go build files inside the project directory, and write the development binary to `bin\samrai-dev.exe`.

## Quick start on Linux or macOS

```bash
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/server
```

Open `http://127.0.0.1:8080/`.

## Docker Compose

```bash
cp .env.example .env
docker compose up --build -d
docker compose logs -f samrai
```

New installations use the named volume `samrai-data`. An older `.env` without `SAMRAI_DOCKER_VOLUME` continues to use the legacy `pageturner-data` volume.

When the service is behind HTTPS, set:

```env
SAMRAI_COOKIE_SECURE=true
```

## Updating an existing installation

Stop the old server and copy its entire `data` directory into the new release. Do not copy only the database: imported originals and prepared comic pages live beside it.

Windows example:

```powershell
New-Item -ItemType Directory -Force "C:\samrai\samrai-v0.2.0-rc.6\data"

Copy-Item "C:\samrai\previous\data\*" `
  "C:\samrai\samrai-v0.2.0-rc.6\data" `
  -Recurse -Force

Copy-Item "C:\samrai\previous\.env" `
  "C:\samrai\samrai-v0.2.0-rc.6\.env" `
  -Force

Set-Location "C:\samrai\samrai-v0.2.0-rc.6"
.\test-windows.bat
.\run-windows.bat
```

Database migrations run on startup. Legacy `PAGETURNER_*` variables, `pageturner.db`, `pageturner.log`, and the old session cookie remain recognized during the rename transition.

Release candidate 5 changes the untouched default managed-library label from `Biblioteca principal` to `Main library`. It does not translate book metadata, series names, annotations, or other user content.

## Data layout

```text
data/
├── config/samrai.db
├── library/          original imported files
├── prepared/         normalized pages for CBR, CB7, and CBT
├── uploads/          temporary incoming files
├── staging/          import and OCR work directories
├── cache/            rebuildable covers, thumbnails, and pages
├── backups/          backups created by the CLI
└── logs/samrai.log
```

## Backup and restore

Administrators can download a full backup from **Settings → Maintenance**.

The same operation is available from the command line:

```bash
go run ./cmd/server backup
```

Restore only while the server is stopped:

```powershell
go run ./cmd/server restore `
  --file "C:\Backups\samrai-backup-20260714-120000.zip" `
  --data-dir ".\data" `
  --force
```

With `--force`, the current database and library are moved to a `pre-restore-*` directory before the backup is installed.

## Commands

```text
samrai                  start the server
samrai version          print version, commit, and build date
samrai healthcheck      check the configured /health endpoint
samrai backup           create a backup under data/backups
samrai restore ...      restore a backup while the server is stopped
```

During development, replace `samrai` with `go run ./cmd/server` or use `run-windows.bat`.

## Frontend development

Run the backend in one terminal:

```powershell
.\run-windows.bat
```

Run Vite in another:

```powershell
Set-Location web
npm ci
npm run dev
```

The development site is available at `http://127.0.0.1:5173/`. To rebuild the files embedded by Go:

```powershell
Set-Location web
npm run typecheck
npm run build
```

## Checks

```bash
go test ./...
go vet ./...

cd web
npm ci
npm run typecheck
npm run build
```

With a local server running, the PowerShell smoke test checks setup, authentication, the library, system information, and backup download:

```powershell
pwsh -ExecutionPolicy Bypass `
  -File .\scripts\release-smoke-test.ps1 `
  -Username samuel
```

## Documentation

- [Installation and upgrades](docs/INSTALLATION.md)
- [Configuration](docs/CONFIGURATION.md)
- [Import pipeline and batch uploads](docs/IMPORTING.md)
- [Comic archives](docs/COMIC_ARCHIVES.md)
- [PDF search, OCR, and annotations](docs/PDF_ANNOTATIONS.md)
- [EPUB support](docs/EPUB.md)
- [Reader behavior](docs/READER.md)
- [Library and series](docs/LIBRARY.md)
- [Favorites, history, and notebooks](docs/ENGAGEMENT.md)
- [Users and authentication](docs/USERS.md)
- [Backup and restore](docs/BACKUP_RESTORE.md)
- [Operations and diagnostics](docs/OPERATIONS.md)
- [Release process](docs/RELEASE.md)

## Known limits

- There is one managed library per installation.
- CBR and CB7 need an external archive tool on native installations.
- Image-only EPUB books do not yet support area annotations.
- OPDS is not implemented.
- Restore is a command-line operation and requires downtime.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. Security reports should follow [SECURITY.md](SECURITY.md).

## License

samrai is released under the [MIT License](LICENSE).

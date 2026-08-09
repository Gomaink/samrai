# Installation and upgrades

## Windows

Extract the source archive to a directory you can write to, then open PowerShell there.

```powershell
Copy-Item .env.example .env

go version
go mod tidy
.\test-windows.bat
.\run-windows.bat
```

The project requires Go 1.26 or newer. Open `http://127.0.0.1:8080/` and create the first administrator account.

The compiled frontend is included in release source archives. Node.js is not required unless you change files under `web`.

## Linux and macOS

```bash
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/server
```

For a long-running native installation, build the binary and run it under the service manager used by your system:

```bash
make build
./bin/samrai
```

## Docker Compose

```bash
cp .env.example .env
docker compose pull
docker compose up -d
docker compose ps
docker compose logs -f samrai
```

Open `http://127.0.0.1:24600/`. The Compose file pulls `ghcr.io/gomaink/samrai:0.2.0-rc.11`, maps host port `24600` to container port `8080`, and stores persistent state in the mounted `/data` volume. The container runs as a non-root user with a read-only root filesystem.

New installations use `samrai-data`. Older `.env` files that do not define `SAMRAI_DOCKER_VOLUME` continue to mount `pageturner-data`.

## Reverse proxy and HTTPS

Point Caddy, Nginx, Traefik, or another reverse proxy at host port 24600. When users connect over HTTPS, set:

```env
SAMRAI_COOKIE_SECURE=true
```

Forward the original scheme and client address according to your proxy's normal configuration. Do not expose the `data` directory as static files.

## Native OCR and archive tools

Tesseract is optional. It is used for scanned PDFs and PDF pages whose text layer cannot be searched reliably.

```env
SAMRAI_TESSERACT_PATH=C:\Program Files\Tesseract-OCR\tesseract.exe
SAMRAI_OCR_WORKERS=1
SAMRAI_OCR_TIMEOUT=90s
```

CBT files are handled internally. Native CBR and CB7 imports require 7-Zip or `lsar` + `unar`.

```env
SAMRAI_7ZIP_PATH=C:\Program Files\7-Zip\7z.exe
```

The Docker image already includes the required OCR and archive packages.

## Upgrade procedure

1. Download a backup.
2. Stop the old server.
3. Extract the new release to a separate directory.
4. Copy the complete old `data` directory.
5. Copy the old `.env` file.
6. Run the test script.
7. Start the new release.
8. Check login, the library, one reader, and one import.

Migrations are applied in a transaction and recorded in `schema_migrations`. Do not run an older samrai build against a database that has already been migrated by a newer build. Restore a pre-upgrade backup instead.

## Windows upgrade example

```powershell
New-Item -ItemType Directory -Force "C:\samrai\samrai-v0.2.0-rc.11\data"
Copy-Item "C:\samrai\samrai-v0.2.0-rc.10\data\*" "C:\samrai\samrai-v0.2.0-rc.11\data" -Recurse -Force
Copy-Item "C:\samrai\samrai-v0.2.0-rc.10\.env" "C:\samrai\samrai-v0.2.0-rc.11\.env" -Force
Set-Location "C:\samrai\samrai-v0.2.0-rc.11"
.\test-windows.bat
.\run-windows.bat
```

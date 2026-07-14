# Configuration

samrai reads environment variables at startup. `run-windows.bat` and `test-windows.bat` load the project `.env` file automatically. Docker Compose reads the same file.

New settings use the `SAMRAI_` prefix. The matching `PAGETURNER_` names are still accepted for upgrades, but should not be used in new installations.

## Server and storage

| Variable | Default | Purpose |
| --- | --- | --- |
| `SAMRAI_ADDRESS` | `:8080` | HTTP listen address |
| `SAMRAI_DATA_DIR` | `./data` | Persistent storage root |
| `SAMRAI_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `SAMRAI_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown deadline |
| `SAMRAI_MAX_DB_CONNECTIONS` | `4` | SQLite connection pool limit |
| `SAMRAI_DOCKER_VOLUME` | `samrai-data` | Named volume used by Compose |

## Sessions and passwords

| Variable | Default | Purpose |
| --- | --- | --- |
| `SAMRAI_SESSION_DURATION` | `720h` | Session lifetime |
| `SAMRAI_COOKIE_SECURE` | `false` | Send the session cookie only over HTTPS |
| `SAMRAI_ARGON2_MEMORY_MIB` | `64` | Argon2id memory cost, 19–1024 MiB |
| `SAMRAI_ARGON2_ITERATIONS` | `3` | Argon2id iteration count, 1–10 |
| `SAMRAI_ARGON2_PARALLELISM` | `2` | Argon2id lanes, 1–16 |

Changing Argon2 values affects newly created password hashes. Existing hashes keep their encoded parameters.

## Import limits

| Variable | Default | Purpose |
| --- | ---: | --- |
| `SAMRAI_MAX_UPLOAD_MIB` | `2048` | Maximum uploaded file size |
| `SAMRAI_MAX_ARCHIVE_MIB` | `8192` | Maximum declared uncompressed archive size |
| `SAMRAI_MAX_PAGE_MIB` | `128` | Maximum image entry size |
| `SAMRAI_MAX_ARCHIVE_ENTRIES` | `10000` | Maximum archive entry count |
| `SAMRAI_MAX_PAGES` | `5000` | Maximum pages in one book |

## Image cache

| Variable | Default | Purpose |
| --- | ---: | --- |
| `SAMRAI_IMAGE_WORKERS` | `2` | Concurrent image conversions, maximum 16 |
| `SAMRAI_IMAGE_CACHE_MIB` | `5120` | Approximate cache size limit |
| `SAMRAI_IMAGE_QUALITY` | `82` | WebP quality, 1–100 |
| `SAMRAI_IMAGE_MAX_WIDTH` | `3840` | Largest generated width, 320–8192 |
| `SAMRAI_CACHE_CLEANUP_INTERVAL` | `1h` | Cache cleanup interval |

The cache is disposable. Deleting it does not remove books or reading data.

## OCR

| Variable | Default | Purpose |
| --- | --- | --- |
| `SAMRAI_TESSERACT_PATH` | auto-detect | Tesseract executable |
| `SAMRAI_OCR_WORKERS` | `1` | Concurrent OCR jobs, maximum 4 |
| `SAMRAI_OCR_TIMEOUT` | `90s` | Per-page OCR deadline |

## Archive tools

| Variable | Default | Purpose |
| --- | --- | --- |
| `SAMRAI_7ZIP_PATH` | auto-detect | 7-Zip executable |
| `SAMRAI_LSAR_PATH` | auto-detect | `lsar` executable |
| `SAMRAI_UNAR_PATH` | auto-detect | `unar` executable |

Set explicit paths only when the programs are installed outside their common locations.

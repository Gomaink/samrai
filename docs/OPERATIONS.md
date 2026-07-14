# Operations and diagnostics

## Health check

`GET /health` does not require authentication. It checks the database connection and returns the service version, commit, database status, and current UTC time.

The CLI can check a running instance:

```bash
go run ./cmd/server healthcheck
```

## Logs

Structured logs are written to standard output and `data/logs/samrai.log`. `SAMRAI_LOG_LEVEL` accepts `debug`, `info`, `warn`, or `error`.

Administrators can view recent log lines from the maintenance screen. Review them before including them in a report because filenames or local paths may appear in operational errors.

## System information

The maintenance screen reports build data, uptime, storage usage, database counts, import limits, worker counts, and cache limits. A support bundle contains diagnostic text and recent logs but not the library or database.

## Metrics and audit records

Authenticated administrators can read system metrics and recent audit events through the API. Audit records cover sensitive account and instance changes; they are not a replacement for host-level logs.

## Temporary directories

`data/uploads` and `data/staging` contain transient work. The server cleans abandoned work when possible and requeues interrupted import jobs at startup. Do not delete these directories while imports or OCR requests are active.

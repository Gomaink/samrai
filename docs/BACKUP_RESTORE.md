# Backup and restore

## Backup contents

A full backup contains:

- the SQLite database;
- original library files;
- prepared CBR, CB7, and CBT pages;
- a manifest with the samrai version and file checksums.

The WebP image cache, temporary uploads, staging directories, logs, and old backup archives are excluded.

## Download from the browser

Sign in as an administrator and open **Settings → Maintenance → Download full backup**. The server creates the archive from a consistent database snapshot and streams it to the browser.

## Create from the command line

```bash
go run ./cmd/server backup
```

The resulting ZIP is written under `data/backups` unless another output path is supplied by the command options.

## Restore

Stop the server first. Restore is intentionally not available from the web interface.

```powershell
go run ./cmd/server restore `
  --file "C:\Backups\samrai-backup-20260714-120000.zip" `
  --data-dir ".\data" `
  --force
```

The restore command validates the manifest, paths, checksums, and expected archive structure. With `--force`, current data is moved to a `pre-restore-*` directory before replacement.

## Test your backups

A backup is not proven until it has been restored. Periodically restore one into a separate data directory, start a temporary server on another port, sign in, and open at least one book from each format you use.

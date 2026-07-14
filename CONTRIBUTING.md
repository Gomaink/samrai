# Contributing

Thanks for taking the time to work on samrai. Small, focused pull requests are easier to review and safer to merge than changes that mix unrelated work.

## Before filing an issue

- Check the latest release candidate.
- Search existing issues.
- Include the format involved: CBZ, CBR, CB7, CBT, PDF, or EPUB.
- Remove private titles, usernames, notes, and file paths from screenshots and logs.

Use private vulnerability reporting instead of a public issue for security problems.

## Development setup

Requirements:

- Go 1.26 or newer;
- Node.js 22 and npm for frontend changes;
- optional Tesseract and archive tools for the related import tests.

```bash
cp .env.example .env
cd web
npm ci
npm run build
cd ..
go mod tidy
go test ./...
go run ./cmd/server
```

On Windows, `test-windows.bat` and `run-windows.bat` set project-local Go cache directories and load `.env`.

## Pull requests

A pull request should:

- explain the problem it solves;
- avoid unrelated formatting changes;
- include tests for behavior changes;
- update the relevant document when setup, configuration, storage, or an API changes;
- keep user-facing text in English;
- pass Go formatting, Go tests, TypeScript checks, and the production frontend build.

Run before submitting:

```bash
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go test ./...
go vet ./...

cd web
npm ci
npm run typecheck
npm run build
```

Do not commit `data`, backups, imported books, `web/node_modules`, or locally built binaries.

## Commit messages

Use a direct subject line that describes the result, for example:

```text
fix duplicate detection for active imports
add keyboard navigation to EPUB reader
```

There is no required commit prefix.

# Security policy

## Supported versions

Security fixes are made against the newest release candidate or stable release. Older preview builds are not maintained.

## Reporting a vulnerability

Please do not open a public issue for a vulnerability that could expose files, credentials, sessions, or another user's reading data.

Use GitHub's private vulnerability reporting feature for the repository. Include:

- the samrai version or commit;
- the deployment method;
- the affected endpoint or file format;
- the steps needed to reproduce the problem;
- the impact you observed;
- a small test file when the issue depends on imported content.

A report does not need a polished proof of concept. Enough detail to reproduce the issue is more useful than a severity label.

## Deployment assumptions

samrai is intended to run on a trusted host behind a reverse proxy. For an Internet-facing deployment:

- terminate TLS at the proxy;
- set `SAMRAI_COOKIE_SECURE=true`;
- restrict access to the data directory and backup files;
- keep the host, container runtime, Tesseract, and archive tools patched;
- do not run the native process as an administrator or root user;
- keep regular offline backups.

The Docker image runs as a non-root user, drops Linux capabilities, and uses a read-only root filesystem. The `/data` volume and temporary filesystem remain writable.

## Imported files

Archive imports reject absolute paths, parent traversal, unsupported entries, excessive entry counts, oversized pages, and archives whose declared uncompressed size exceeds the configured limit. Originals are stored under content-derived names rather than client-supplied paths.

Tesseract and external archive tools process untrusted content. Run the service with the least privilege it needs and keep those tools current.

## Private data

Reading progress, favorites, sessions, highlights, notes, and area annotations are scoped to the authenticated user. Full backups contain that data and the original library files. Treat backup archives as sensitive.

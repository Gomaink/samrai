# Release process

## First publication on GitHub

Create an empty repository named `samrai`. Do not add a README, license, or `.gitignore` from the GitHub form because those files already exist in this project.

From the extracted release directory:

```powershell
git init
git add .
git commit -m "release: samrai v0.2.0-rc.7"
git branch -M main
git remote add origin https://github.com/<github-user>/samrai.git
git push -u origin main
```

Wait for the `CI` workflow to pass before creating the release tag:

```powershell
git tag -a v0.2.0-rc.7 -m "samrai v0.2.0-rc.7"
git push origin v0.2.0-rc.7
```

The tag starts the `Release` workflow. It builds and publishes the container image, creates the source archives and checksum manifest, and opens a GitHub prerelease using `RELEASE_NOTES.md`.

## Before tagging

1. Update the version in the Makefile, Dockerfile, Compose file, Go version package, frontend package files, README, changelog, and release notes.
2. Run Go formatting and verify that no file changes afterward.
3. Run Go tests and vet.
4. Install frontend dependencies from the lockfile, type-check, and build.
5. Run the smoke tests against a clean temporary data directory.
6. Upgrade a copy of data from the previous release candidate.
7. Open at least one comic, PDF, fixed-layout EPUB, and reflowable EPUB.
8. Check the source archives and their SHA-256 manifest.

## Local checks

```bash
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go test ./...
go vet ./...

cd web
npm ci
npm run typecheck
npm run build
```

## Build metadata

The Makefile injects version, commit, and UTC build time through Go linker flags. Docker accepts the same values as build arguments and writes them into OCI labels.

## Tags

Release tags use the form:

```text
v0.2.0-rc.7
v0.2.0
```

Tags containing `rc`, `preview`, `alpha`, or `beta` are prereleases. They must not replace the stable Docker `latest` tag.

## GitHub Actions

The release workflow builds source archives named:

```text
samrai-v<version>-source.zip
samrai-v<version>-source.tar.gz
```

It writes both hashes to a SHA-256 manifest and marks prerelease tags accordingly. Compare the generated asset names with the manifest before publishing the release.

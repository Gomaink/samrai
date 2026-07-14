warning: in the working copy of '.github/ISSUE_TEMPLATE/bug_report.yml', LF will be replaced by CRLF the next time Git touches it
warning: in the working copy of '.github/workflows/release.yml', LF will be replaced by CRLF the next time Git touches it
warning: in the working copy of 'Dockerfile', LF will be replaced by CRLF the next time Git touches it
warning: in the working copy of 'Makefile', LF will be replaced by CRLF the next time Git touches it
warning: in the working copy of 'compose.yaml', LF will be replaced by CRLF the next time Git touches it
warning: in the working copy of 'web/package-lock.json', LF will be replaced by CRLF the next time Git touches it
warning: in the working copy of 'web/package.json', LF will be replaced by CRLF the next time Git touches it
[1mdiff --git a/.github/ISSUE_TEMPLATE/bug_report.yml b/.github/ISSUE_TEMPLATE/bug_report.yml[m
[1mindex b9ae8a6..afcf4a7 100644[m
[1m--- a/.github/ISSUE_TEMPLATE/bug_report.yml[m
[1m+++ b/.github/ISSUE_TEMPLATE/bug_report.yml[m
[36m@@ -13,7 +13,7 @@[m [mbody:[m
     attributes:[m
       label: samrai version[m
       description: Use the value shown in Settings or `samrai version`.[m
[31m-      placeholder: 0.2.0-rc.5[m
[32m+[m[32m      placeholder: 0.2.0-rc.6[m
     validations:[m
       required: true[m
   - type: dropdown[m
[1mdiff --git a/.github/workflows/release.yml b/.github/workflows/release.yml[m
[1mindex fa809a9..bd24a2e 100644[m
[1m--- a/.github/workflows/release.yml[m
[1m+++ b/.github/workflows/release.yml[m
[36m@@ -64,7 +64,8 @@[m [mjobs:[m
           sha256sum "samrai-v${version}-source.zip" "samrai-v${version}-source.tar.gz" > SHA256SUMS.txt[m
       - uses: softprops/action-gh-release@v2[m
         with:[m
[31m-          generate_release_notes: true[m
[32m+[m[32m          name: samrai v${{ steps.meta.outputs.version }}[m
[32m+[m[32m          body_path: RELEASE_NOTES.md[m
           prerelease: ${{ steps.meta.outputs.prerelease == 'true' }}[m
           files: |[m
             samrai-v${{ steps.meta.outputs.version }}-source.zip[m
[1mdiff --git a/CHANGELOG.md b/CHANGELOG.md[m
[1mindex 0905ab9..f6ef688 100644[m
[1m--- a/CHANGELOG.md[m
[1m+++ b/CHANGELOG.md[m
[36m@@ -2,6 +2,13 @@[m
 [m
 This file records user-visible changes. Dates use UTC.[m
 [m
[32m+[m[32m## [0.2.0-rc.6] - 2026-07-14[m
[32m+[m
[32m+[m[32m### Fixed[m
[32m+[m
[32m+[m[32m- Repaired two PDF catalog tests whose English fixtures no longer matched the behavior they were intended to verify.[m
[32m+[m[32m- Kept the metadata limit test focused on Unicode rune boundaries and the excerpt test focused on matched-position trimming.[m
[32m+[m
 ## [0.2.0-rc.5] - 2026-07-14[m
 [m
 ### Changed[m
[1mdiff --git a/Dockerfile b/Dockerfile[m
[1mindex 766dbbd..7b2ebd0 100644[m
[1m--- a/Dockerfile[m
[1m+++ b/Dockerfile[m
[36m@@ -1,6 +1,6 @@[m
 # syntax=docker/dockerfile:1[m
 [m
[31m-ARG VERSION=0.2.0-rc.5[m
[32m+[m[32mARG VERSION=0.2.0-rc.6[m
 ARG COMMIT=source[m
 ARG BUILD_DATE=unknown[m
 [m
[1mdiff --git a/Makefile b/Makefile[m
[1mindex 1e94884..6f62c12 100644[m
[1m--- a/Makefile[m
[1m+++ b/Makefile[m
[36m@@ -1,5 +1,5 @@[m
 APP := samrai[m
[31m-VERSION ?= 0.2.0-rc.5[m
[32m+[m[32mVERSION ?= 0.2.0-rc.6[m
 COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)[m
 BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)[m
 LDFLAGS := -s -w \[m
[1mdiff --git a/README.md b/README.md[m
[1mindex 43d6f7a..4b5db83 100644[m
[1m--- a/README.md[m
[1m+++ b/README.md[m
[36m@@ -2,7 +2,13 @@[m
 [m
 samrai is a self-hosted library and reader for manga, comics, PDFs, and EPUB books. It runs as a single Go server with the web application embedded in the binary and stores its state in SQLite.[m
 [m
[31m-> **Release status:** `v0.2.0-rc.5` is a release candidate. Back up the `data` directory before upgrading and report anything that blocks normal reading or library management.[m
[32m+[m[32m> **Release status:** `v0.2.0-rc.6` is a release candidate. Back up the `data` directory before upgrading and report anything that blocks normal reading or library management.[m
[32m+[m
[32m+[m[32m<p align="center">[m
[32m+[m[32m  <a href="https://imgur.com/FkzDTBn">[m
[32m+[m[32m    <img src="https://i.imgur.com/FkzDTBn.png" alt="samrai library interface" width="1100">[m
[32m+[m[32m  </a>[m
[32m+[m[32m</p>[m
 [m
 ## What it does[m
 [m
[36m@@ -91,17 +97,17 @@[m [mStop the old server and copy its entire `data` directory into the new release. D[m
 Windows example:[m
 [m
 ```powershell[m
[31m-New-Item -ItemType Directory -Force "C:\samrai\samrai-v0.2.0-rc.5\data"[m
[32m+[m[32mNew-Item -ItemType Directory -Force "C:\samrai\samrai-v0.2.0-rc.6\data"[m
 [m
 Copy-Item "C:\samrai\previous\data\*" `[m
[31m-  "C:\samrai\samrai-v0.2.0-rc.5\data" `[m
[32m+[m[32m  "C:\samrai\samrai-v0.2.0-rc.6\data" `[m
   -Recurse -Force[m
 [m
 Copy-Item "C:\samrai\previous\.env" `[m
[31m-  "C:\samrai\samrai-v0.2.0-rc.5\.env" `[m
[32m+[m[32m  "C:\samrai\samrai-v0.2.0-rc.6\.env" `[m
   -Force[m
 [m
[31m-Set-Location "C:\samrai\samrai-v0.2.0-rc.5"[m
[32m+[m[32mSet-Location "C:\samrai\samrai-v0.2.0-rc.6"[m
 .\test-windows.bat[m
 .\run-windows.bat[m
 ```[m
[1mdiff --git a/RELEASE_NOTES.md b/RELEASE_NOTES.md[m
[1mindex 453fc2b..ebdffb0 100644[m
[1m--- a/RELEASE_NOTES.md[m
[1m+++ b/RELEASE_NOTES.md[m
[36m@@ -1,52 +1,27 @@[m
[31m-# samrai v0.2.0-rc.5[m
[32m+[m[32m# samrai v0.2.0-rc.6[m
 [m
[31m-Release candidate 5 prepares the repository for public development and makes English the project language.[m
[32m+[m[32mRelease candidate 6 fixes two test fixtures introduced while translating the repository to English.[m
 [m
[31m-## English interface and server output[m
[32m+[m[32m## What was wrong[m
 [m
[31m-The browser interface, API error messages, command output, logs, scripts, test fixtures, and default instance labels are now in English. Existing titles, series names, notes, usernames, and other user-created data are left as they are.[m
[32m+[m[32mThe application code was behaving correctly, but two PDF catalog tests no longer represented their original cases:[m
 [m
[31m-A migration changes the managed library name from `Biblioteca principal` to `Main library` only when the old value is still the untouched default. Custom library names are not changed.[m
[32m+[m[32m- the metadata limit test expected the word `title` after truncating a longer English phrase to six runes;[m
[32m+[m[32m- the excerpt test searched for `montreal` and then required the original accented text `Montréal` to contain the unaccented spelling.[m
 [m
[31m-## Repository work[m
[32m+[m[32m## Fix[m
 [m
[31m-This release adds the files normally expected in a public repository:[m
[32m+[m[32mThe metadata test now uses `résumé` to verify trimming and Unicode rune boundaries. The excerpt test now uses an unaccented matched term, while accent-insensitive search remains covered by its dedicated test.[m
 [m
[31m-- an MIT license;[m
[31m-- contribution and conduct guidelines;[m
[31m-- issue forms and a pull request template;[m
[31m-- Dependabot configuration;[m
[31m-- EditorConfig and Git attributes;[m
[31m-- a configuration reference;[m
[31m-- installation, development, release, and operational documentation in English.[m
[32m+[m[32mNo database migration or runtime behavior changes are included in this release.[m
 [m
[31m-The README now covers native Windows, native Unix, Docker, upgrades, backups, frontend development, and release checks from a clean checkout.[m
[32m+[m[32m## Upgrade from rc.5[m
 [m
[31m-## Compatibility[m
[32m+[m[32m1. Stop rc.5.[m
[32m+[m[32m2. Extract rc.6 to a new directory.[m
[32m+[m[32m3. Copy the complete rc.5 `data` directory.[m
[32m+[m[32m4. Copy the rc.5 `.env` file.[m
[32m+[m[32m5. Run `test-windows.bat`.[m
[32m+[m[32m6. Start rc.6.[m
 [m
[31m-The rename compatibility introduced in earlier candidates remains in place. Existing installations may continue to use:[m
[31m-[m
[31m-- `PAGETURNER_*` environment variables;[m
[31m-- `pageturner.db` and `pageturner.log`;[m
[31m-- the legacy session cookie;[m
[31m-- the Docker volume `pageturner-data`.[m
[31m-[m
[31m-New configuration should use the `SAMRAI_*` names.[m
[31m-[m
[31m-## Upgrade from rc.4[m
[31m-[m
[31m-1. Stop rc.4.[m
[31m-2. Extract rc.5 to a new directory.[m
[31m-3. Copy the complete rc.4 `data` directory.[m
[31m-4. Copy the rc.4 `.env` file.[m
[31m-5. Run the test script.[m
[31m-6. Start rc.5 and check login, one comic, one PDF or EPUB, and the import screen.[m
[31m-[m
[31m-No imported file or user content needs to be renamed.[m
[31m-[m
[31m-## Still known[m
[31m-[m
[31m-- This is a release candidate, not the final `0.2.0` release.[m
[31m-- Restores still require the server to be stopped.[m
[31m-- Native CBR and CB7 imports require 7-Zip or `lsar` + `unar`.[m
[31m-- DRM-protected EPUB files are not supported.[m
[32m+[m[32mAll library data and settings remain compatible.[m
[1mdiff --git a/compose.yaml b/compose.yaml[m
[1mindex 5a77a29..524d4b6 100644[m
[1m--- a/compose.yaml[m
[1m+++ b/compose.yaml[m
[36m@@ -1,10 +1,10 @@[m
 services:[m
   samrai:[m
[31m-    image: samrai:0.2.0-rc.5[m
[32m+[m[32m    image: samrai:0.2.0-rc.6[m
     build:[m
       context: .[m
       args:[m
[31m-        VERSION: 0.2.0-rc.5[m
[32m+[m[32m        VERSION: 0.2.0-rc.6[m
         COMMIT: source[m
         BUILD_DATE: local[m
     container_name: samrai[m
[1mdiff --git a/docs/RELEASE.md b/docs/RELEASE.md[m
[1mindex 79d3ea0..7ab43a8 100644[m
[1m--- a/docs/RELEASE.md[m
[1m+++ b/docs/RELEASE.md[m
[36m@@ -1,5 +1,29 @@[m
 # Release process[m
 [m
[32m+[m[32m## First publication on GitHub[m
[32m+[m
[32m+[m[32mCreate an empty repository named `samrai`. Do not add a README, license, or `.gitignore` from the GitHub form because those files already exist in this project.[m
[32m+[m
[32m+[m[32mFrom the extracted release directory:[m
[32m+[m
[32m+[m[32m```powershell[m
[32m+[m[32mgit init[m
[32m+[m[32mgit add .[m
[32m+[m[32mgit commit -m "release: samrai v0.2.0-rc.6"[m
[32m+[m[32mgit branch -M main[m
[32m+[m[32mgit remote add origin https://github.com/<github-user>/samrai.git[m
[32m+[m[32mgit push -u origin main[m
[32m+[m[32m```[m
[32m+[m
[32m+[m[32mWait for the `CI` workflow to pass before creating the release tag:[m
[32m+[m
[32m+[m[32m```powershell[m
[32m+[m[32mgit tag -a v0.2.0-rc.6 -m "samrai v0.2.0-rc.6"[m
[32m+[m[32mgit push origin v0.2.0-rc.6[m
[32m+[m[32m```[m
[32m+[m
[32m+[m[32mThe tag starts the `Release` workflow. It builds and publishes the container image, creates the source archives and checksum manifest, and opens a GitHub prerelease using `RELEASE_NOTES.md`.[m
[32m+[m
 ## Before tagging[m
 [m
 1. Update the version in the Makefile, Dockerfile, Compose file, Go version package, frontend package files, README, changelog, and release notes.[m
[36m@@ -33,7 +57,7 @@[m [mThe Makefile injects version, commit, and UTC build time through Go linker flags[m
 Release tags use the form:[m
 [m
 ```text[m
[31m-v0.2.0-rc.5[m
[32m+[m[32mv0.2.0-rc.6[m
 v0.2.0[m
 ```[m
 [m
[1mdiff --git a/internal/catalog/pdf_analysis_test.go b/internal/catalog/pdf_analysis_test.go[m
[1mindex d06a155..dd279c0 100644[m
[1m--- a/internal/catalog/pdf_analysis_test.go[m
[1m+++ b/internal/catalog/pdf_analysis_test.go[m
[36m@@ -13,8 +13,8 @@[m [mfunc TestNormalizePDFText(t *testing.T) {[m
 }[m
 [m
 func TestLimitPDFMetadata(t *testing.T) {[m
[31m-	got := limitPDFMetadata("  very long title  ", 6)[m
[31m-	if want := "title"; got != want {[m
[32m+[m	[32mgot := limitPDFMetadata("  résumé too long  ", 6)[m
[32m+[m	[32mif want := "résumé"; got != want {[m
 		t.Fatalf("limitPDFMetadata() = %q, want %q", got, want)[m
 	}[m
 }[m
[36m@@ -26,9 +26,9 @@[m [mfunc TestCountFold(t *testing.T) {[m
 }[m
 [m
 func TestPDFSearchExcerptUsesMatchedPosition(t *testing.T) {[m
[31m-	text := strings.Repeat("before ", 30) + "Montréal" + strings.Repeat(" after", 30)[m
[31m-	got := pdfSearchExcerpt(text, "montreal")[m
[31m-	if !strings.Contains(strings.ToLower(got), "montreal") {[m
[32m+[m	[32mtext := strings.Repeat("before ", 30) + "New York" + strings.Repeat(" after", 30)[m
[32m+[m	[32mgot := pdfSearchExcerpt(text, "york")[m
[32m+[m	[32mif !strings.Contains(strings.ToLower(got), "york") {[m
 		t.Fatalf("pdfSearchExcerpt() did not retain query: %q", got)[m
 	}[m
 	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {[m
[1mdiff --git a/internal/version/version.go b/internal/version/version.go[m
[1mindex ea779e7..65134e4 100644[m
[1m--- a/internal/version/version.go[m
[1m+++ b/internal/version/version.go[m
[36m@@ -1,7 +1,7 @@[m
 package version[m
 [m
 var ([m
[31m-	Version = "0.2.0-rc.5"[m
[32m+[m	[32mVersion = "0.2.0-rc.6"[m
 	Commit  = "source"[m
 	Date    = "2026-07-14"[m
 )[m
[1mdiff --git a/scripts/release-smoke-test.ps1 b/scripts/release-smoke-test.ps1[m
[1mindex 9e6b1c7..daa3d6b 100644[m
[1m--- a/scripts/release-smoke-test.ps1[m
[1m+++ b/scripts/release-smoke-test.ps1[m
[36m@@ -50,7 +50,7 @@[m [mtry {[m
         }[m
     }[m
 [m
[31m-    Write-Host "v0.2.0-rc.5 smoke test complete." -ForegroundColor Green[m
[32m+[m[32m    Write-Host "v0.2.0-rc.6 smoke test complete." -ForegroundColor Green[m
 }[m
 finally {[m
     $password = $null[m
[1mdiff --git a/web/package-lock.json b/web/package-lock.json[m
[1mindex 0e78127..de50ce8 100644[m
[1m--- a/web/package-lock.json[m
[1m+++ b/web/package-lock.json[m
[36m@@ -1,12 +1,12 @@[m
 {[m
   "name": "samrai-web",[m
[31m-  "version": "0.2.0-rc.5",[m
[32m+[m[32m  "version": "0.2.0-rc.6",[m
   "lockfileVersion": 3,[m
   "requires": true,[m
   "packages": {[m
     "": {[m
       "name": "samrai-web",[m
[31m-      "version": "0.2.0-rc.5",[m
[32m+[m[32m      "version": "0.2.0-rc.6",[m
       "dependencies": {[m
         "@tanstack/react-query": "5.101.2",[m
         "pdfjs-dist": "6.1.200",[m
[1mdiff --git a/web/package.json b/web/package.json[m
[1mindex 414cb95..61e9c15 100644[m
[1m--- a/web/package.json[m
[1m+++ b/web/package.json[m
[36m@@ -1,7 +1,7 @@[m
 {[m
   "name": "samrai-web",[m
   "private": true,[m
[31m-  "version": "0.2.0-rc.5",[m
[32m+[m[32m  "version": "0.2.0-rc.6",[m
   "type": "module",[m
   "scripts": {[m
     "dev": "vite",[m

# Imports and batch uploads

## Accepted files

samrai imports CBZ/ZIP, CBR/RAR, CB7/7z, CBT/TAR, PDF, and DRM-free EPUB files.

## Import pipeline

1. The administrator selects one or more files in the browser.
2. Each upload is written to `data/uploads` under a random temporary name.
3. The server calculates SHA-256 and rejects a file already present in the library or an active job.
4. A persistent import job is created.
5. A worker validates the file, extracts metadata, and prepares any required pages.
6. The original moves to `data/library` under a content-derived filename.
7. The book becomes visible after the job reaches `completed`.

Interrupted `running` jobs return to `pending` on the next startup.

## Batch import

The upload dialog accepts multiple files and folder selection. Before sending a batch, it shows editable title, series, and volume fields for every item.

Filename parsing recognizes common forms such as:

```text
Berserk - Volume 01.pdf
Berserk Vol. 02.pdf
Berserk v03.pdf
Berserk 04.pdf
```

The browser sorts names naturally, so volume 2 comes before volume 10. At most two uploads run at once. Import processing continues in the server-side queue and does not depend on the dialog remaining open.

Suggestions are only a starting point. Review them before upload, especially when filenames contain edition numbers, years, or chapter ranges.

## Comic archives

CBZ files are read directly after validation. CBR, CB7, and CBT imports write normalized pages to `data/prepared/<hash>/pages`, which avoids reopening solid archives for every page request.

Comic archive processing:

- rejects unsafe paths and unsupported entry types;
- accepts JPEG, PNG, and WebP pages;
- orders page names naturally;
- reads `ComicInfo.xml` when present;
- uses the first image as the fallback cover;
- keeps the original archive.

CBT is handled internally. CBR and CB7 require 7-Zip or `lsar` + `unar` on native installations.

## PDF and EPUB

PDF originals stay intact. The browser performs the fast classification and sends extracted metadata, cover information, and searchable text according to the document's OCR policy.

EPUB import reads the container, package document, manifest, spine, navigation, metadata, and cover. Fixed-layout and reflowable books use different readers but share the same library record.

## Limits

The relevant environment variables are documented in [Configuration](CONFIGURATION.md). Defaults allow files up to 2 GiB, archives declaring up to 8 GiB after extraction, 10,000 entries, and 5,000 pages per book.

Job states are `pending`, `running`, `completed`, `failed`, and `cancelled`.

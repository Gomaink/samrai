# Comic archives

## Formats

- CBZ is a ZIP archive.
- CBR is usually a RAR archive.
- CB7 is a 7z archive.
- CBT is a TAR archive.

Pages may be JPEG, PNG, or WebP. Directory entries, `ComicInfo.xml`, and supported images are the only archive members used by the reader.

## Prepared storage

CBZ pages can be opened efficiently from the validated ZIP. CBR, CB7, and CBT are normalized once into:

```text
data/prepared/<file-hash>/pages/
```

Prepared pages are part of full backups because restored solid archives should open immediately. The WebP response cache is separate and can be rebuilt.

## External tools

CBT needs no external program. CBR and CB7 use one of these toolsets:

- 7-Zip;
- `lsar` and `unar`.

samrai checks common install locations. Use `SAMRAI_7ZIP_PATH`, `SAMRAI_LSAR_PATH`, or `SAMRAI_UNAR_PATH` when automatic detection is not enough.

## Validation

The importer rejects:

- absolute paths and `..` traversal;
- symbolic links and unsupported special entries;
- unsupported page types;
- files larger than the configured page limit;
- excessive entry counts or page counts;
- archives whose declared extracted size exceeds the configured limit.

Extraction happens in a staging directory. A failed import does not publish a partial book.

## ComicInfo.xml

When present, `ComicInfo.xml` can supply title, series, volume or number, summary, writer, publisher, year, language, reading direction, and page metadata. An administrator can correct imported metadata from the book details screen.

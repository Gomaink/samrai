# Library, search, and series

## Managed library

A new installation creates one managed library under `data/library`. Imported originals are named from their SHA-256 hash; the filename supplied by the browser remains metadata.

The database stores relative paths where possible, so moving or restoring the whole `data` directory does not require rewriting every book record.

## Home screen

The dashboard shows total counts, books in progress, recent additions, and a sample of series. Reading progress and favorites are calculated for the signed-in user.

## Book list

The library can be searched and filtered by format, progress, favorites, and series. Sorting includes recent, oldest, title, series and volume, progress, and publication year.

Search covers the title, series, writer, publisher, and original filename fields used by the catalog query.

## Series

Books with the same normalized series record are grouped. A series page shows its cover, volume count, started count, completed count, total pages, and books in series order.

Changing a book's series creates or reuses the matching series record. Removing the series value makes the book standalone.

## Deleting a book

Book deletion is restricted to administrators. The catalog record, stored original, prepared pages, cached images, progress, and related annotations are removed through the service layer. Keep a backup when deletion must be reversible.

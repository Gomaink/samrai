# PDF search and annotations

## Import analysis

After upload, the browser inspects the PDF with PDF.js and sends basic metadata, cover information, page count, and a document classification to the server. The classification helps decide whether native text is useful and whether OCR should remain off, run on demand, or be enabled more broadly.

## Search

Search uses stored native text and OCR text. Queries shorter than two normalized characters are ignored. Matches are grouped by page and include an excerpt and source kind.

For damaged native text, samrai can repair a limited set of split tokens before matching. It still enforces word boundaries so a query does not match inside an unrelated word.

## Protected PDFs

A protected document asks for its password in the reader. The password is held in the browser while the book is open and is not written to the database or logs.

## Highlights and notes

Text selections can become highlights with an optional private note. The annotation stores the selected text and a structured anchor used to relocate the mark.

Area annotations store page-relative rectangles and are useful for image-only pages. Coordinates are normalized so the mark follows the page at different display sizes.

Users can edit notes, change colors, remove annotations, and export one book or the combined notebook as Markdown.

## Privacy and storage

PDF annotations are scoped by user and book. Other readers cannot see them. Deleting the source book removes its associated page text and annotations.

## Practical limits

Text selection quality depends on the mapping supplied by the PDF. Some embedded fonts provide visible glyphs without usable Unicode text; those documents need OCR or area annotations. Rotated text, complex multi-column layouts, and unusual writing directions can also reduce selection accuracy.

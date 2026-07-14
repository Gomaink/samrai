# PDF OCR

## Why OCR is selective

Many manga PDFs are image-only and can contain hundreds of pages. Running OCR across every page during import would make imports slow, consume significant CPU, and create text that may never be searched. samrai classifies the document first and applies the selected OCR policy.

## Modes

- **Off:** keep native PDF text only.
- **On demand:** run OCR for a requested page when search or reading needs it.
- **Background:** allow broader indexing for documents where OCR is useful.

The administrator can change the mode on a PDF after import.

## Search behavior

Native text and OCR text share normalized search behavior. Search is case-insensitive and accent-insensitive, matches whole words and phrases, and produces excerpts around the match.

Limited token repair is used for native PDF text where font encoding splits a word into fragments. OCR output is not silently joined in the same way because that can create false matches.

## Windows

Install Tesseract and point samrai at the executable when it is not found automatically:

```env
SAMRAI_TESSERACT_PATH=C:\Program Files\Tesseract-OCR\tesseract.exe
SAMRAI_OCR_WORKERS=1
SAMRAI_OCR_TIMEOUT=90s
```

Install the language data needed by your library. The Docker image includes English and Portuguese data.

## Resource limits

OCR runs in a staging directory with a per-page timeout and a small worker pool. Increasing `SAMRAI_OCR_WORKERS` can make indexing faster but also increases CPU and memory pressure. The configured maximum is four workers.

Failed OCR does not remove the PDF. The page remains readable and the error can be retried after correcting the Tesseract installation.

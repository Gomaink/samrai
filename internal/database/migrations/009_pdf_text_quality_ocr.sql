ALTER TABLE books ADD COLUMN pdf_ocr_status TEXT NOT NULL DEFAULT 'not_needed';
ALTER TABLE books ADD COLUMN pdf_invalid_pages INTEGER NOT NULL DEFAULT 0 CHECK (pdf_invalid_pages >= 0);
ALTER TABLE books ADD COLUMN pdf_ocr_pages INTEGER NOT NULL DEFAULT 0 CHECK (pdf_ocr_pages >= 0);

ALTER TABLE pdf_pages ADD COLUMN text_source TEXT NOT NULL DEFAULT 'pdf';
ALTER TABLE pdf_pages ADD COLUMN text_quality REAL NOT NULL DEFAULT 0;

-- The previous index accepted corrupt character maps as searchable text. Rebuild
-- every PDF with the quality detector introduced in preview.2-fix2.
DELETE FROM pdf_pages;

UPDATE books
SET pdf_analysis_status = 'pending',
    pdf_text_layer = 'unknown',
    pdf_ocr_status = 'idle',
    pdf_invalid_pages = 0,
    pdf_ocr_pages = 0,
    pdf_analyzed_at = NULL,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE format = 'pdf' AND status = 'ready';

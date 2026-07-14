ALTER TABLE books ADD COLUMN pdf_document_kind TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE books ADD COLUMN pdf_classification_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE books ADD COLUMN pdf_ocr_mode TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE books ADD COLUMN pdf_sampled_pages INTEGER NOT NULL DEFAULT 0 CHECK (pdf_sampled_pages >= 0);
ALTER TABLE books ADD COLUMN pdf_indexed_pages INTEGER NOT NULL DEFAULT 0 CHECK (pdf_indexed_pages >= 0);

UPDATE books
SET pdf_classification_status = CASE
        WHEN pdf_analysis_status = 'complete' THEN 'complete'
        ELSE 'pending'
    END,
    pdf_document_kind = CASE pdf_text_layer
        WHEN 'text' THEN 'text'
        WHEN 'ocr' THEN 'scanned_book'
        WHEN 'hybrid' THEN 'mixed'
        WHEN 'mixed' THEN 'mixed'
        WHEN 'scanned' THEN 'scanned_book'
        ELSE 'unknown'
    END,
    pdf_ocr_mode = CASE
        WHEN pdf_text_layer = 'text' THEN 'off'
        WHEN pdf_ocr_status = 'complete' THEN 'full'
        ELSE 'auto'
    END,
    pdf_indexed_pages = (
        SELECT COUNT(*)
        FROM pdf_pages p
        WHERE p.book_id = books.id AND p.text_content <> ''
    )
WHERE format = 'pdf';

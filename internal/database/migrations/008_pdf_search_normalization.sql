ALTER TABLE pdf_pages ADD COLUMN search_content TEXT NOT NULL DEFAULT '';
ALTER TABLE pdf_pages ADD COLUMN search_compact TEXT NOT NULL DEFAULT '';

-- The extraction algorithm changed in preview.2-fix1. Existing indexes were
-- created by inserting a space between every PDF.js text item, which can split
-- a visible word into fragments such as "Digi tal". Rebuild them lazily the
-- next time each PDF is opened.
DELETE FROM pdf_pages;

UPDATE books
SET pdf_analysis_status = 'pending',
    pdf_analyzed_at = NULL,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE format = 'pdf' AND status = 'ready';

ALTER TABLE books ADD COLUMN pdf_analysis_status TEXT NOT NULL DEFAULT 'not_applicable';
ALTER TABLE books ADD COLUMN pdf_text_layer TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE books ADD COLUMN pdf_analyzed_at TEXT;

UPDATE books
SET pdf_analysis_status = 'pending'
WHERE format = 'pdf' AND pdf_analysis_status = 'not_applicable';

CREATE TABLE pdf_pages (
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL CHECK (page_number >= 0),
    text_content TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY(book_id, page_number)
);

CREATE INDEX pdf_pages_book_page_idx ON pdf_pages(book_id, page_number);

CREATE TABLE pdf_covers (
    book_id INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
    media_type TEXT NOT NULL CHECK (media_type IN ('image/webp', 'image/jpeg', 'image/png')),
    width INTEGER NOT NULL CHECK (width > 0),
    height INTEGER NOT NULL CHECK (height > 0),
    image_data BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

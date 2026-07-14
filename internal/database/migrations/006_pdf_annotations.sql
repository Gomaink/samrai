ALTER TABLE books ADD COLUMN format TEXT NOT NULL DEFAULT 'cbz';

CREATE INDEX IF NOT EXISTS books_format_idx ON books(format);

CREATE TABLE annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL CHECK (page_number >= 0),
    kind TEXT NOT NULL CHECK (kind IN ('highlight', 'area')),
    color TEXT NOT NULL DEFAULT 'yellow'
        CHECK (color IN ('yellow', 'green', 'blue', 'pink', 'orange')),
    selected_text TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    anchor_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX annotations_user_book_page_idx
    ON annotations(user_id, book_id, page_number, created_at);
CREATE INDEX annotations_book_idx ON annotations(book_id);

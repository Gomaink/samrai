CREATE TABLE epub_annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    spine_index INTEGER NOT NULL CHECK (spine_index >= 0),
    resource_path TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'highlight'
        CHECK (kind IN ('highlight', 'note')),
    color TEXT NOT NULL DEFAULT 'yellow'
        CHECK (color IN ('yellow', 'green', 'blue', 'pink', 'orange')),
    selected_text TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    anchor_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(book_id, spine_index) REFERENCES epub_spine(book_id, spine_index) ON DELETE CASCADE
);

CREATE INDEX epub_annotations_user_book_spine_idx
    ON epub_annotations(user_id, book_id, spine_index, created_at);
CREATE INDEX epub_annotations_book_idx ON epub_annotations(book_id);

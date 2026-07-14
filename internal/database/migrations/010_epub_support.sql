ALTER TABLE books ADD COLUMN epub_layout TEXT NOT NULL DEFAULT 'not_applicable';
ALTER TABLE books ADD COLUMN epub_version TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN epub_package_path TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN epub_cover_path TEXT NOT NULL DEFAULT '';

ALTER TABLE reading_progress ADD COLUMN location_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE epub_resources (
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    resource_path TEXT NOT NULL,
    item_id TEXT NOT NULL DEFAULT '',
    media_type TEXT NOT NULL,
    properties TEXT NOT NULL DEFAULT '',
    file_size INTEGER NOT NULL DEFAULT 0 CHECK (file_size >= 0),
    PRIMARY KEY(book_id, resource_path)
);

CREATE INDEX epub_resources_book_media_idx
    ON epub_resources(book_id, media_type);

CREATE TABLE epub_spine (
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    spine_index INTEGER NOT NULL CHECK (spine_index >= 0),
    item_id TEXT NOT NULL,
    resource_path TEXT NOT NULL,
    media_type TEXT NOT NULL,
    properties TEXT NOT NULL DEFAULT '',
    linear INTEGER NOT NULL DEFAULT 1 CHECK (linear IN (0, 1)),
    title TEXT NOT NULL DEFAULT '',
    text_content TEXT NOT NULL DEFAULT '',
    search_content TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(book_id, spine_index),
    FOREIGN KEY(book_id, resource_path) REFERENCES epub_resources(book_id, resource_path) ON DELETE CASCADE
);

CREATE INDEX epub_spine_book_path_idx
    ON epub_spine(book_id, resource_path);

CREATE TABLE epub_toc (
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    label TEXT NOT NULL,
    resource_path TEXT NOT NULL,
    fragment TEXT NOT NULL DEFAULT '',
    spine_index INTEGER,
    depth INTEGER NOT NULL DEFAULT 0 CHECK (depth >= 0),
    PRIMARY KEY(book_id, position)
);

CREATE INDEX epub_toc_book_spine_idx
    ON epub_toc(book_id, spine_index, position);

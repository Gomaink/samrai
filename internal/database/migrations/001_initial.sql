CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'reader')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE libraries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    management_mode TEXT NOT NULL DEFAULT 'managed'
        CHECK (management_mode IN ('managed', 'external')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE series (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    reading_direction TEXT
        CHECK (reading_direction IS NULL OR reading_direction IN ('ltr', 'rtl')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(library_id, title)
);

CREATE INDEX series_library_id_idx ON series(library_id);

CREATE TABLE books (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    series_id INTEGER REFERENCES series(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    file_path TEXT NOT NULL UNIQUE,
    original_filename TEXT NOT NULL,
    file_size INTEGER NOT NULL CHECK (file_size >= 0),
    file_hash TEXT NOT NULL,
    page_count INTEGER NOT NULL DEFAULT 0 CHECK (page_count >= 0),
    status TEXT NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing', 'ready', 'failed')),
    reading_direction TEXT
        CHECK (reading_direction IS NULL OR reading_direction IN ('ltr', 'rtl')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX books_library_id_idx ON books(library_id);
CREATE INDEX books_series_id_idx ON books(series_id);
CREATE INDEX books_status_idx ON books(status);
CREATE UNIQUE INDEX books_file_hash_idx ON books(file_hash);

CREATE TABLE pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL CHECK (page_number >= 0),
    archive_path TEXT NOT NULL,
    media_type TEXT NOT NULL,
    width INTEGER NOT NULL CHECK (width > 0),
    height INTEGER NOT NULL CHECK (height > 0),
    file_size INTEGER NOT NULL CHECK (file_size >= 0),
    page_hash TEXT,
    UNIQUE(book_id, page_number),
    UNIQUE(book_id, archive_path)
);

CREATE INDEX pages_book_id_idx ON pages(book_id);

CREATE TABLE reading_progress (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    current_page INTEGER NOT NULL DEFAULT 0 CHECK (current_page >= 0),
    completed INTEGER NOT NULL DEFAULT 0 CHECK (completed IN (0, 1)),
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at TEXT,
    PRIMARY KEY(user_id, book_id)
);

CREATE INDEX reading_progress_updated_at_idx ON reading_progress(updated_at DESC);

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    payload TEXT NOT NULL DEFAULT '{}',
    error_message TEXT,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    started_at TEXT,
    completed_at TEXT
);

CREATE INDEX jobs_status_created_at_idx ON jobs(status, created_at);

CREATE TABLE user_preferences (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme TEXT NOT NULL DEFAULT 'system'
        CHECK (theme IN ('light', 'dark', 'oled', 'system')),
    reader_background TEXT NOT NULL DEFAULT 'black',
    reading_direction TEXT NOT NULL DEFAULT 'ltr'
        CHECK (reading_direction IN ('ltr', 'rtl')),
    page_fit TEXT NOT NULL DEFAULT 'contain'
        CHECK (page_fit IN ('contain', 'width', 'height', 'original')),
    page_layout TEXT NOT NULL DEFAULT 'single'
        CHECK (page_layout IN ('single', 'double', 'auto')),
    preload_count INTEGER NOT NULL DEFAULT 2 CHECK (preload_count BETWEEN 0 AND 5)
);

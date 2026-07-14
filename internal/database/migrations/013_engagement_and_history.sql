CREATE TABLE book_favorites (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY(user_id, book_id)
);

CREATE INDEX book_favorites_user_created_idx
    ON book_favorites(user_id, created_at DESC);

CREATE TABLE series_favorites (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY(user_id, series_id)
);

CREATE INDEX series_favorites_user_created_idx
    ON series_favorites(user_id, created_at DESC);

CREATE TABLE reading_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    session_key TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_activity_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ended_at TEXT,
    start_page INTEGER NOT NULL DEFAULT 0 CHECK (start_page >= 0),
    end_page INTEGER NOT NULL DEFAULT 0 CHECK (end_page >= 0),
    start_location_json TEXT NOT NULL DEFAULT '{}',
    end_location_json TEXT NOT NULL DEFAULT '{}',
    duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    UNIQUE(user_id, session_key)
);

CREATE INDEX reading_sessions_user_activity_idx
    ON reading_sessions(user_id, last_activity_at DESC, id DESC);
CREATE INDEX reading_sessions_book_idx
    ON reading_sessions(book_id, last_activity_at DESC);

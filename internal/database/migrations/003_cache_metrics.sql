CREATE TABLE cache_statistics (
    cache_name TEXT PRIMARY KEY,
    hits INTEGER NOT NULL DEFAULT 0 CHECK (hits >= 0),
    misses INTEGER NOT NULL DEFAULT 0 CHECK (misses >= 0),
    generated INTEGER NOT NULL DEFAULT 0 CHECK (generated >= 0),
    generated_bytes INTEGER NOT NULL DEFAULT 0 CHECK (generated_bytes >= 0),
    generation_milliseconds INTEGER NOT NULL DEFAULT 0 CHECK (generation_milliseconds >= 0),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO cache_statistics(cache_name) VALUES ('images')
ON CONFLICT(cache_name) DO NOTHING;

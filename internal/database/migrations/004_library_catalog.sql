CREATE INDEX IF NOT EXISTS books_created_at_idx ON books(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS books_title_idx ON books(title COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS books_writer_idx ON books(writer COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS series_title_idx ON series(title COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS reading_progress_user_updated_idx ON reading_progress(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS reading_progress_user_completed_idx ON reading_progress(user_id, completed, updated_at DESC);

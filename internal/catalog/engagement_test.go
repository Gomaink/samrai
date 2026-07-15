package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"samrai/internal/database"
)

func newEngagementTestService(t *testing.T) (*Service, int64, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "engagement.db"), 1)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	userResult, err := db.ExecContext(ctx, `INSERT INTO users(username,password_hash,role) VALUES('reader','hash','reader')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := userResult.LastInsertId()
	libraryResult, err := db.ExecContext(ctx, `INSERT INTO libraries(name,path) VALUES('Main','library')`)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	libraryID, _ := libraryResult.LastInsertId()
	seriesResult, err := db.ExecContext(ctx, `INSERT INTO series(library_id,title) VALUES(?, 'Series')`, libraryID)
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}
	seriesID, _ := seriesResult.LastInsertId()

	insertBook := func(title, path, hash string) int64 {
		result, err := db.ExecContext(ctx, `
			INSERT INTO books(library_id,series_id,title,file_path,original_filename,file_size,file_hash,page_count,status,format)
			VALUES(?,?,?,?,?,1,?,20,'ready','pdf')
		`, libraryID, seriesID, title, path, title+".pdf", hash)
		if err != nil {
			t.Fatalf("insert book %s: %v", title, err)
		}
		id, _ := result.LastInsertId()
		return id
	}

	firstBookID := insertBook("Book A", "library/a.pdf", "hash-a")
	secondBookID := insertBook("Book B", "library/b.pdf", "hash-b")
	return NewService(db), userID, firstBookID, secondBookID, seriesID
}

func TestFavoritesAreScopedToUser(t *testing.T) {
	service, userID, bookID, _, seriesID := newEngagementTestService(t)
	ctx := context.Background()

	if err := service.SetBookFavorite(ctx, userID, bookID, true); err != nil {
		t.Fatalf("favorite book: %v", err)
	}
	if err := service.SetSeriesFavorite(ctx, userID, seriesID, true); err != nil {
		t.Fatalf("favorite series: %v", err)
	}

	favorites, err := service.Favorites(ctx, userID)
	if err != nil {
		t.Fatalf("load favorites: %v", err)
	}
	if len(favorites.Books) != 1 || favorites.Books[0].ID != bookID {
		t.Fatalf("unexpected favorite books: %+v", favorites.Books)
	}
	if len(favorites.Series) != 1 || favorites.Series[0].ID != seriesID {
		t.Fatalf("unexpected favorite series: %+v", favorites.Series)
	}
}

func TestReadingSessionCannotMoveBetweenBooks(t *testing.T) {
	service, userID, firstBookID, secondBookID, _ := newEngagementTestService(t)
	ctx := context.Background()
	const sessionKey = "session-cross-book-0001"

	if err := service.RecordReadingActivity(ctx, userID, firstBookID, sessionKey, 3, json.RawMessage(`{"page":3}`)); err != nil {
		t.Fatalf("record first activity: %v", err)
	}
	if err := service.RecordReadingActivity(ctx, userID, secondBookID, sessionKey, 9, json.RawMessage(`{"page":9}`)); !errors.Is(err, ErrReadingSessionBookMismatch) {
		t.Fatalf("cross-book activity error = %v, want %v", err, ErrReadingSessionBookMismatch)
	}

	history, err := service.ListReadingHistory(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history) != 1 || history[0].BookID != firstBookID || history[0].EndPage != 3 {
		t.Fatalf("session was reassigned or changed: %+v", history)
	}

	if err := service.EndReadingSession(ctx, userID, secondBookID, sessionKey); err != nil {
		t.Fatalf("end wrong book session: %v", err)
	}
	history, err = service.ListReadingHistory(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("list history after wrong end: %v", err)
	}
	if history[0].EndedAt != nil {
		t.Fatal("ending the same key for another book ended the original session")
	}

	if err := service.EndReadingSession(ctx, userID, firstBookID, sessionKey); err != nil {
		t.Fatalf("end correct session: %v", err)
	}
	history, err = service.ListReadingHistory(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("list history after end: %v", err)
	}
	if history[0].EndedAt == nil {
		t.Fatal("correct session was not ended")
	}
}

func TestListAllAnnotationsCombinesPDFAndEPUB(t *testing.T) {
	service, userID, pdfBookID, epubBookID, _ := newEngagementTestService(t)
	ctx := context.Background()
	db := service.db

	if _, err := db.ExecContext(ctx, `UPDATE books SET format='epub' WHERE id=?`, epubBookID); err != nil {
		t.Fatalf("set EPUB format: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO epub_resources(book_id,resource_path,item_id,media_type) VALUES(?,'chapter.xhtml','chapter','application/xhtml+xml')`, epubBookID); err != nil {
		t.Fatalf("insert EPUB resource: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO epub_spine(book_id,spine_index,item_id,resource_path,media_type,title,linear) VALUES(?,0,'chapter','chapter.xhtml','application/xhtml+xml','Chapter',1)`, epubBookID); err != nil {
		t.Fatalf("insert spine: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO annotations(user_id,book_id,page_number,kind,color,selected_text,note,anchor_json) VALUES(?,?,2,'highlight','yellow','PDF excerpt','PDF note','{}')`, userID, pdfBookID); err != nil {
		t.Fatalf("insert PDF annotation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO epub_annotations(user_id,book_id,spine_index,resource_path,kind,color,selected_text,note,anchor_json) VALUES(?,?,0,'chapter.xhtml','highlight','blue','EPUB excerpt','EPUB note','{}')`, userID, epubBookID); err != nil {
		t.Fatalf("insert EPUB annotation: %v", err)
	}

	items, total, err := service.ListAllAnnotations(ctx, userID, AnnotationListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list annotations: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total=%d len=%d, want 2", total, len(items))
	}
	sources := map[string]bool{}
	for _, item := range items {
		sources[item.Source] = true
	}
	if !sources["pdf"] || !sources["epub"] {
		t.Fatalf("missing source in annotations: %+v", items)
	}

	filtered, filteredTotal, err := service.ListAllAnnotations(ctx, userID, AnnotationListOptions{Search: "Book B", Limit: 10})
	if err != nil {
		t.Fatalf("search annotations by book title: %v", err)
	}
	if filteredTotal != 1 || len(filtered) != 1 || filtered[0].BookID != epubBookID || filtered[0].BookTitle != "Book B" {
		t.Fatalf("book title search returned total=%d items=%+v", filteredTotal, filtered)
	}
}

func TestSetCompletionPreservesReadingPosition(t *testing.T) {
	service, userID, firstBookID, secondBookID, _ := newEngagementTestService(t)
	ctx := context.Background()

	progress, err := service.SaveProgress(ctx, userID, firstBookID, 7, json.RawMessage(`{"page":7,"zoom":1.25}`))
	if err != nil {
		t.Fatalf("save progress: %v", err)
	}
	if progress.Completed {
		t.Fatal("progress was completed before the manual update")
	}

	updated, err := service.SetCompletion(ctx, userID, []int64{firstBookID, secondBookID, firstBookID}, true)
	if err != nil {
		t.Fatalf("mark books as read: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2 unique books", updated)
	}

	first, err := service.GetBook(ctx, userID, firstBookID)
	if err != nil {
		t.Fatalf("get first book: %v", err)
	}
	if !first.Completed || first.CurrentPage != 7 || string(first.ReadingLocation) != `{"page":7,"zoom":1.25}` {
		t.Fatalf("manual completion changed the saved position: %+v", first)
	}
	second, err := service.GetBook(ctx, userID, secondBookID)
	if err != nil {
		t.Fatalf("get second book: %v", err)
	}
	if !second.Started || !second.Completed || second.CurrentPage != 0 {
		t.Fatalf("unstarted book was not marked as read: %+v", second)
	}

	updated, err = service.SetCompletion(ctx, userID, []int64{firstBookID, secondBookID}, false)
	if err != nil {
		t.Fatalf("mark books as unread: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	first, err = service.GetBook(ctx, userID, firstBookID)
	if err != nil {
		t.Fatalf("get first book after unread: %v", err)
	}
	if first.Completed || first.CurrentPage != 7 || string(first.ReadingLocation) != `{"page":7,"zoom":1.25}` {
		t.Fatalf("mark as unread did not preserve the saved position: %+v", first)
	}
}

func TestSetCompletionRejectsMissingBookAtomically(t *testing.T) {
	service, userID, firstBookID, _, _ := newEngagementTestService(t)
	ctx := context.Background()

	if _, err := service.SetCompletion(ctx, userID, []int64{firstBookID, 999999}, true); !errors.Is(err, ErrBookNotFound) {
		t.Fatalf("SetCompletion error = %v, want %v", err, ErrBookNotFound)
	}
	book, err := service.GetBook(ctx, userID, firstBookID)
	if err != nil {
		t.Fatalf("get book: %v", err)
	}
	if book.Started || book.Completed {
		t.Fatalf("partial completion was committed: %+v", book)
	}
}

package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestExportAllAnnotationsIncludesMoreThanOnePageAndGroupsBooks(t *testing.T) {
	server := newTestServer(t)
	setup := performRequest(t, server, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "samuel",
		"password": "a-secure-password",
	}, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", setup.Code, setup.Body.String())
	}
	cookie := responseCookie(t, setup, sessionCookieName)

	var userID int64
	if err := server.db.QueryRow(`SELECT id FROM users WHERE username = 'samuel'`).Scan(&userID); err != nil {
		t.Fatalf("load user: %v", err)
	}
	libraryResult, err := server.db.Exec(`INSERT INTO libraries(name,path) VALUES('Main','library')`)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	libraryID, _ := libraryResult.LastInsertId()
	insertBook := func(title, fileName, hash string) int64 {
		result, err := server.db.Exec(`
			INSERT INTO books(library_id,title,file_path,original_filename,file_size,file_hash,page_count,status,format)
			VALUES(?,?,?,?,1,?,700,'ready','pdf')
		`, libraryID, title, "library/"+fileName, fileName, hash)
		if err != nil {
			t.Fatalf("insert book %s: %v", title, err)
		}
		id, _ := result.LastInsertId()
		return id
	}
	bookA := insertBook("Book A", "a.pdf", "export-hash-a")
	bookB := insertBook("Book B", "b.pdf", "export-hash-b")

	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin annotations: %v", err)
	}
	statement, err := tx.Prepare(`INSERT INTO annotations(user_id,book_id,page_number,kind,color,selected_text,note,anchor_json) VALUES(?,?,?,'highlight','yellow',?,?,'{}')`)
	if err != nil {
		t.Fatalf("prepare annotations: %v", err)
	}
	for index := 0; index < 501; index++ {
		if _, err := statement.Exec(userID, bookA, index, fmt.Sprintf("excerpt-%03d", index), fmt.Sprintf("note-%03d", index)); err != nil {
			t.Fatalf("insert annotation %d: %v", index, err)
		}
	}
	if _, err := statement.Exec(userID, bookB, 0, "book-b-quote", "book-b-note"); err != nil {
		t.Fatalf("insert annotation for book B: %v", err)
	}
	if err := statement.Close(); err != nil {
		t.Fatalf("close statement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit annotations: %v", err)
	}

	response := performRequest(t, server, http.MethodGet, "/api/v1/annotations/export", nil, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("export status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{"# samrai annotations", "note-000", "note-500", "book-b-note"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("export is missing %q", expected)
		}
	}
	if count := strings.Count(body, "\n## Book A\n"); count != 1 {
		t.Fatalf("Book A heading count = %d, want 1", count)
	}
	if count := strings.Count(body, "\n## Book B\n"); count != 1 {
		t.Fatalf("Book B heading count = %d, want 1", count)
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="samrai-annotations.md"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
}

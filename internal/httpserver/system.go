package httpserver

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"samrai/internal/auth"
)

type auditEventResponse struct {
	ID        int64  `json:"id"`
	Actor     string `json:"actor"`
	EventType string `json:"event_type"`
	Subject   string `json:"subject"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}
type auditEventsResponse struct {
	Items []auditEventResponse `json:"items"`
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	v, e := s.system.Info(r.Context())
	if e != nil {
		writeError(w, 500, "internal_error", "Could not load system information.")
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) recentLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil {
			limit = n
		}
	}
	v, e := s.system.RecentLogs(limit)
	if e != nil {
		writeError(w, 500, "internal_error", "Could not read server logs.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	path, name, e := s.backups.Create(r.Context())
	if e != nil {
		s.logger.Error("create backup", "error", e)
		writeError(w, 500, "backup_failed", "Could not create the backup.")
		return
	}
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
		if _, auditErr := s.db.ExecContext(r.Context(), `INSERT INTO audit_events(actor_user_id,event_type,subject_type,subject_id,detail) VALUES(?,'backup.created','backup',?,?)`, principal.User.ID, name, "download from web interface"); auditErr != nil {
			s.logger.Warn("record backup audit event", "error", auditErr)
		}
	}
	defer os.Remove(path)
	f, e := os.Open(path)
	if e != nil {
		writeError(w, 500, "backup_failed", "Could not open the backup.")
		return
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		writeError(w, 500, "backup_failed", "Could not inspect the backup.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, st.ModTime(), f)
}
func (s *Server) supportBundle(w http.ResponseWriter, r *http.Request) {
	info, e := s.system.Info(r.Context())
	if e != nil {
		writeError(w, 500, "diagnostics_failed", "Could not create the support bundle.")
		return
	}
	logs, e := s.system.RecentLogs(300)
	if e != nil {
		writeError(w, 500, "diagnostics_failed", "Could not read server logs.")
		return
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	entry, _ := zw.Create("diagnostics.json")
	_ = json.NewEncoder(entry).Encode(map[string]any{"info": info, "logs": logs})
	readme, _ := zw.Create("README.txt")
	_, _ = readme.Write([]byte("samrai support bundle. It contains no passwords, session tokens, database, or book contents. Logs may contain local paths, book IDs, filenames, titles, and error messages; review the archive before sharing.\n"))
	if e := zw.Close(); e != nil {
		writeError(w, 500, "diagnostics_failed", "Could not finalize the support bundle.")
		return
	}
	name := "samrai-diagnostics-" + time.Now().UTC().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b.Bytes())
}
func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request) {
	rows, e := s.db.QueryContext(r.Context(), `SELECT e.id,COALESCE(u.username,'system'),e.event_type,CASE WHEN e.subject_type='' THEN '' ELSE e.subject_type||':'||e.subject_id END,e.detail,e.created_at FROM audit_events e LEFT JOIN users u ON u.id=e.actor_user_id ORDER BY e.created_at DESC,e.id DESC LIMIT 100`)
	if e != nil {
		writeError(w, 500, "internal_error", "Could not load reading history.")
		return
	}
	defer rows.Close()
	items := make([]auditEventResponse, 0)
	for rows.Next() {
		var x auditEventResponse
		if e := rows.Scan(&x.ID, &x.Actor, &x.EventType, &x.Subject, &x.Detail, &x.CreatedAt); e != nil {
			writeError(w, 500, "internal_error", "Could not load reading history.")
			return
		}
		items = append(items, x)
	}
	writeJSON(w, 200, auditEventsResponse{items})
}

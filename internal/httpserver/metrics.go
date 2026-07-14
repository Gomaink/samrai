package httpserver

import (
	"net/http"
	"runtime"
	"time"

	"samrai/internal/imagecache"
)

type runtimeMetrics struct {
	UptimeSeconds   int64  `json:"uptime_seconds"`
	Goroutines      int    `json:"goroutines"`
	HeapBytes       uint64 `json:"heap_bytes"`
	HeapInUseBytes  uint64 `json:"heap_in_use_bytes"`
	SystemBytes     uint64 `json:"system_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	GarbageCycles   uint32 `json:"garbage_cycles"`
}

type systemMetricsResponse struct {
	Runtime runtimeMetrics     `json:"runtime"`
	Images  imagecache.Metrics `json:"images"`
}

func (s *Server) systemMetrics(w http.ResponseWriter, _ *http.Request) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	var images imagecache.Metrics
	if s.images != nil {
		images = s.images.Metrics()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, systemMetricsResponse{
		Runtime: runtimeMetrics{
			UptimeSeconds: int64(time.Since(s.startedAt).Seconds()), Goroutines: runtime.NumGoroutine(),
			HeapBytes: memory.HeapAlloc, HeapInUseBytes: memory.HeapInuse,
			SystemBytes: memory.Sys, TotalAllocBytes: memory.TotalAlloc, GarbageCycles: memory.NumGC,
		},
		Images: images,
	})
}

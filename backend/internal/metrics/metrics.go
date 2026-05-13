package metrics

import (
	"database/sql"
	"runtime"
	"time"
)

// Snapshot holds lightweight runtime and DB stats useful for capacity planning.
type Snapshot struct {
	Timestamp     time.Time `json:"timestamp"`
	NumGoroutine  int       `json:"num_goroutine"`
	NumCPU        int       `json:"num_cpu"`
	GoVersion     string    `json:"go_version"`
	MemAlloc      uint64    `json:"mem_alloc_bytes"`
	MemTotalAlloc uint64    `json:"mem_total_alloc_bytes"`
	MemSys        uint64    `json:"mem_sys_bytes"`
	MemHeapAlloc  uint64    `json:"mem_heap_alloc_bytes"`
	MemHeapSys    uint64    `json:"mem_heap_sys_bytes"`
	MemNumGC      uint32    `json:"mem_num_gc"`
	DBOpenConns   int       `json:"db_open_conns,omitempty"`
	DBInUse       int       `json:"db_in_use,omitempty"`
	DBIdleConns   int       `json:"db_idle_conns,omitempty"`
	DBWaitCount   int64     `json:"db_wait_count,omitempty"`
}

// Collect returns a Snapshot of current process and DB stats. DB may be nil.
func Collect(db *sql.DB) Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s := Snapshot{
		Timestamp:     time.Now().UTC(),
		NumGoroutine:  runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		GoVersion:     runtime.Version(),
		MemAlloc:      ms.Alloc,
		MemTotalAlloc: ms.TotalAlloc,
		MemSys:        ms.Sys,
		MemHeapAlloc:  ms.HeapAlloc,
		MemHeapSys:    ms.HeapSys,
		MemNumGC:      ms.NumGC,
	}
	if db != nil {
		st := db.Stats()
		s.DBOpenConns = st.OpenConnections
		s.DBInUse = st.InUse
		s.DBIdleConns = st.Idle
		s.DBWaitCount = st.WaitCount
	}
	return s
}

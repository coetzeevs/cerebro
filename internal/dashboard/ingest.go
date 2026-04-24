package dashboard

import (
	"os"
	"path/filepath"

	"github.com/coetzeevs/cerebro/internal/metrics"
)

// ingestFromDir performs an incremental ingest of JSONL files from the given directory.
// This is called on dashboard launch and on each live-refresh tick.
// It mirrors the logic in cmd_ingest.go but operates directly on a MetricsStore.
func ingestFromDir(store *metrics.MetricsStore, sessionDir string) {
	files, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		return
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		state, _ := store.GetIngestState(file)
		if state.LastOffset >= info.Size() {
			continue // file hasn't grown
		}

		result, newOffset, err := metrics.ParseJSONL(file)
		if err != nil || len(result.Turns) == 0 {
			continue
		}

		for _, sid := range result.SessionIDs() {
			_ = store.DeleteToolCallsForSession(sid)
		}
		_ = store.InsertTurnMetrics(result.Turns)
		if len(result.ToolCalls) > 0 {
			_ = store.InsertToolCalls(result.ToolCalls)
		}
		_ = store.SetIngestState(file, newOffset)
	}
}

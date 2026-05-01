package coordinator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/cmd184psu/ctrq/internal/worker"
)

func (c *Coordinator) handleExecutionOutput(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	execID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	stdout, stderr, found := worker.Registry.Get(execID)
	if !found {
		// execution not in memory — check DB for status
		exec, dbErr := c.db.GetExecution(execID)
		if dbErr != nil || exec == nil {
			writeError(w, http.StatusNotFound, "execution not found")
			return
		}
		fmt.Fprintf(w, "event: status\ndata: %s\n\n", exec.Status)
		flusher.Flush()
		return
	}

	// Replay buffered stdout lines
	for _, line := range stdout.Lines() {
		sendSSELine(w, line)
		flusher.Flush()
	}
	// Replay buffered stderr lines
	for _, line := range stderr.Lines() {
		sendSSELine(w, line)
		flusher.Flush()
	}

	if stdout.Done() && stderr.Done() {
		fmt.Fprintf(w, "event: done\ndata: {}\n\n")
		flusher.Flush()
		return
	}

	// Subscribe to live output
	stdoutCh := stdout.Subscribe()
	stderrCh := stderr.Subscribe()

	for {
		select {
		case line, ok := <-stdoutCh:
			if !ok {
				stdoutCh = nil
			} else {
				sendSSELine(w, line)
				flusher.Flush()
			}
		case line, ok := <-stderrCh:
			if !ok {
				stderrCh = nil
			} else {
				sendSSELine(w, line)
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
		if stdoutCh == nil && stderrCh == nil {
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return
		}
	}
}

func sendSSELine(w http.ResponseWriter, line worker.OutputLine) {
	data, err := json.Marshal(map[string]string{
		"stream": line.Stream,
		"line":   line.Line,
		"ts":     line.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
	})
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: output\ndata: %s\n\n", data)
}

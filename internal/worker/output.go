package worker

import (
	"bytes"
	"sync"
	"time"
)

const ringSize = 1000

type OutputLine struct {
	Line      string    `json:"line"`
	Timestamp time.Time `json:"ts"`
	Stream    string    `json:"stream"`
}

// OutputCapture captures stdout/stderr into a ring buffer and fans out to SSE subscribers.
type OutputCapture struct {
	mu          sync.RWMutex
	lines       [ringSize]OutputLine
	head        int
	total       int
	done        bool
	subscribers []chan OutputLine
	ExecID      int64
	stream      string
}

func NewOutputCapture(execID int64, stream string) *OutputCapture {
	return &OutputCapture{ExecID: execID, stream: stream}
}

// Write implements io.Writer. Splits on newlines and stores each line.
func (oc *OutputCapture) Write(p []byte) (int, error) {
	lines := bytes.Split(p, []byte("\n"))
	oc.mu.Lock()
	defer oc.mu.Unlock()
	for _, l := range lines {
		if len(l) == 0 {
			continue
		}
		ol := OutputLine{Line: string(l), Timestamp: time.Now(), Stream: oc.stream}
		oc.lines[oc.head%ringSize] = ol
		oc.head++
		oc.total++
		for _, ch := range oc.subscribers {
			select {
			case ch <- ol:
			default:
			}
		}
	}
	return len(p), nil
}

// Subscribe returns a channel that receives future lines. The caller must drain it.
func (oc *OutputCapture) Subscribe() <-chan OutputLine {
	ch := make(chan OutputLine, 256)
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.subscribers = append(oc.subscribers, ch)
	return ch
}

// MarkDone signals execution finished; closes all subscriber channels.
func (oc *OutputCapture) MarkDone() {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.done = true
	for _, ch := range oc.subscribers {
		close(ch)
	}
	oc.subscribers = nil
}

// Done reports whether the execution has finished.
func (oc *OutputCapture) Done() bool {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	return oc.done
}

// Lines returns all buffered lines in order (oldest first).
func (oc *OutputCapture) Lines() []OutputLine {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	if oc.total == 0 {
		return nil
	}
	count := oc.total
	if count > ringSize {
		count = ringSize
	}
	out := make([]OutputLine, count)
	start := oc.head - count
	if start < 0 {
		start = 0
	}
	for i := 0; i < count; i++ {
		out[i] = oc.lines[(start+i)%ringSize]
	}
	return out
}

// captureSet holds both stdout and stderr captures for one execution.
type captureSet struct {
	Stdout    *OutputCapture
	Stderr    *OutputCapture
	CreatedAt time.Time
}

// OutputRegistry maps execID to its capture set.
type OutputRegistry struct {
	mu       sync.RWMutex
	captures map[int64]*captureSet
}

var Registry = &OutputRegistry{captures: make(map[int64]*captureSet)}

func (r *OutputRegistry) Register(execID int64) (*OutputCapture, *OutputCapture) {
	stdout := NewOutputCapture(execID, "stdout")
	stderr := NewOutputCapture(execID, "stderr")
	r.mu.Lock()
	r.captures[execID] = &captureSet{Stdout: stdout, Stderr: stderr, CreatedAt: time.Now()}
	r.mu.Unlock()
	return stdout, stderr
}

func (r *OutputRegistry) Get(execID int64) (*OutputCapture, *OutputCapture, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cs, ok := r.captures[execID]
	if !ok {
		return nil, nil, false
	}
	return cs.Stdout, cs.Stderr, true
}

func (r *OutputRegistry) Unregister(execID int64) {
	r.mu.Lock()
	delete(r.captures, execID)
	r.mu.Unlock()
}

// StartGC starts a background goroutine that removes completed captures older than ttl.
func (r *OutputRegistry) StartGC(ttl time.Duration) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			r.gc(ttl)
		}
	}()
}

func (r *OutputRegistry) gc(ttl time.Duration) {
	cutoff := time.Now().Add(-ttl)
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, cs := range r.captures {
		if cs.CreatedAt.Before(cutoff) && cs.Stdout.Done() && cs.Stderr.Done() {
			delete(r.captures, id)
		}
	}
}

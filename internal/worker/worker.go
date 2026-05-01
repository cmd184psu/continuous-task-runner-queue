package worker

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"time"

	"github.com/cmd184psu/ctrq/internal/db"
	"github.com/cmd184psu/ctrq/internal/models"
)

const pollInterval = 5 * time.Second
const lockTTL = 10 * time.Minute

type Worker struct {
	db       *db.DB
	cfg      *models.Config
	executor Executor
	workerID string
}

func New(database *db.DB, cfg *models.Config, workerID string) *Worker {
	return &Worker{
		db:       database,
		cfg:      cfg,
		executor: &TaskExecutor{},
		workerID: workerID,
	}
}

// NewWithExecutor allows injecting a mock executor for testing.
func NewWithExecutor(database *db.DB, cfg *models.Config, workerID string, exec Executor) *Worker {
	return &Worker{db: database, cfg: cfg, executor: exec, workerID: workerID}
}

func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.poll(); err != nil {
				log.Printf("worker poll error: %v", err)
			}
		}
	}
}

func (w *Worker) poll() error {
	if err := w.db.CleanupExpiredLocks(); err != nil {
		log.Printf("cleanup locks: %v", err)
	}

	eligible, err := w.db.GetEligibleTasks()
	if err != nil {
		return err
	}
	if len(eligible) == 0 {
		return nil
	}

	groups, err := w.db.ListGroups()
	if err != nil {
		return err
	}

	for _, group := range groups {
		running, err := w.db.CountRunningInGroup(group.Name)
		if err != nil {
			log.Printf("count running in %s: %v", group.Name, err)
			continue
		}
		slots := group.PoolLimit - running
		if slots <= 0 {
			continue
		}

		var candidates []*models.Task
		for _, t := range eligible {
			if t.GroupName == group.Name {
				candidates = append(candidates, t)
			}
		}

		toRun := min(slots, len(candidates))
		for i := 0; i < toRun; i++ {
			task := candidates[i]
			ok, err := w.db.AcquireLock(task.ID, w.workerID, lockTTL)
			if err != nil || !ok {
				continue
			}
			now := time.Now()
			// Reuse an existing pending execution (e.g. from EnqueueTask) if
			// one exists, rather than creating a second pending record that
			// would be orphaned and cause infinite re-runs.
			var execID int64
			pending, perr := w.db.GetPendingExecution(task.ID)
			if perr != nil {
				w.db.ReleaseLock(task.ID, w.workerID)
				log.Printf("get pending execution for %s: %v", task.Name, perr)
				continue
			}
			if pending != nil {
				execID = pending.ID
			} else {
				execID, err = w.db.CreateExecution(task.ID, w.workerID, now)
				if err != nil {
					w.db.ReleaseLock(task.ID, w.workerID)
					log.Printf("create execution for %s: %v", task.Name, err)
					continue
				}
			}
			go w.runTask(task, execID, now)
		}
	}
	return nil
}

func (w *Worker) runTask(task *models.Task, execID int64, scheduledAt time.Time) {
	stdout, stderr := Registry.Register(execID)
	defer func() {
		go func() {
			time.Sleep(time.Hour)
			Registry.Unregister(execID)
		}()
	}()

	var stdoutW, stderrW io.Writer = stdout, stderr
	if task.OutputFile != "" {
		path := strings.ReplaceAll(task.OutputFile, "{exec_id}", strconv.FormatInt(execID, 10))
		path = strings.ReplaceAll(path, "{task}", task.Name)
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Printf("create output dir %s: %v", dir, err)
			}
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("open output file %s: %v", path, err)
		} else {
			defer f.Close()
			fmt.Fprintf(f, "--- exec %d  task %s  started %s ---\n",
				execID, task.Name, time.Now().Format(time.RFC3339))
			stdoutW = io.MultiWriter(stdout, f)
			stderrW = io.MultiWriter(stderr, f)
		}
	}

	startedAt := time.Now()
	if err := w.db.StartExecution(execID, w.workerID); err != nil {
		log.Printf("start execution %d: %v", execID, err)
	}

	err := w.executor.Execute(task, stdoutW, stderrW)

	stdout.MarkDone()
	stderr.MarkDone()

	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	schedDelay := startedAt.Sub(scheduledAt).Milliseconds()

	status := "success"
	var errMsg *string
	if err != nil {
		status = "failed"
		s := err.Error()
		errMsg = &s
	}

	if ferr := w.db.FinishExecution(execID, status, errMsg, durationMs, schedDelay); ferr != nil {
		log.Printf("finish execution %d: %v", execID, ferr)
	}
	if merr := w.db.RecordMetric(task.ID, execID, status, durationMs, schedDelay); merr != nil {
		log.Printf("record metric for task %d: %v", task.ID, merr)
	}
	if lerr := w.db.ReleaseLock(task.ID, w.workerID); lerr != nil {
		log.Printf("release lock for task %d: %v", task.ID, lerr)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

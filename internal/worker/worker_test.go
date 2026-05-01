package worker_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/cmd184psu/ctrq/internal/db"
	"github.com/cmd184psu/ctrq/internal/models"
	"github.com/cmd184psu/ctrq/internal/worker"
	"github.com/stretchr/testify/require"
)

type mockExecutor struct {
	fn func(task *models.Task, stdout, stderr io.Writer) error
}

func (m *mockExecutor) Execute(task *models.Task, stdout, stderr io.Writer) error {
	return m.fn(task, stdout, stderr)
}

func successExec() *mockExecutor {
	return &mockExecutor{fn: func(task *models.Task, stdout, stderr io.Writer) error {
		stdout.Write([]byte("done\n"))
		return nil
	}}
}

func newTestWorker(t *testing.T, exec worker.Executor) (*worker.Worker, *db.DB) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	cfg := &models.Config{
		Groups: []models.GroupConfig{
			{Name: "test", PoolLimit: 2},
		},
	}
	err = d.UpsertGroup(&models.Group{Name: "test", PoolLimit: 2, AllowedTypes: []string{}})
	require.NoError(t, err)

	w := worker.NewWithExecutor(d, cfg, "test-worker", exec)
	return w, d
}

func addTask(t *testing.T, d *db.DB, name string) *models.Task {
	t.Helper()
	task := &models.Task{
		Name: name, GroupName: "test", TaskType: "shell",
		Enabled: true, Priority: 50, Args: `{"shell":"echo hi"}`,
	}
	id, err := d.AddTask(task)
	require.NoError(t, err)
	task.ID = id
	return task
}

func TestWorker_PicksEligibleTask(t *testing.T) {
	executed := make(chan string, 1)
	exec := &mockExecutor{fn: func(task *models.Task, stdout, stderr io.Writer) error {
		executed <- task.Name
		return nil
	}}

	w, d := newTestWorker(t, exec)
	addTask(t, d, "my-task")

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	select {
	case name := <-executed:
		require.Equal(t, "my-task", name)
	case <-time.After(15 * time.Second):
		t.Fatal("task was not executed within timeout")
	}
	cancel()
}

func TestWorker_RespectsPoolLimit(t *testing.T) {
	started := make(chan string, 10)
	release := make(chan struct{})
	exec := &mockExecutor{fn: func(task *models.Task, stdout, stderr io.Writer) error {
		started <- task.Name
		<-release
		return nil
	}}

	_, d := newTestWorker(t, exec)
	w2 := worker.NewWithExecutor(d, &models.Config{
		Groups: []models.GroupConfig{{Name: "test", PoolLimit: 2}},
	}, "w2", exec)

	addTask(t, d, "task-a")
	addTask(t, d, "task-b")
	addTask(t, d, "task-c")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w2.Start(ctx)

	// Wait for 2 to start
	<-started
	<-started

	// Third should not start yet
	select {
	case <-started:
		t.Fatal("pool limit exceeded — third task started while two were running")
	case <-time.After(7 * time.Second):
		// correct: no third task started
	}
	close(release)
}

func TestWorker_SkipsPausedGroup(t *testing.T) {
	executed := make(chan string, 1)
	exec := &mockExecutor{fn: func(task *models.Task, stdout, stderr io.Writer) error {
		executed <- task.Name
		return nil
	}}

	w, d := newTestWorker(t, exec)
	addTask(t, d, "task-in-paused-group")
	require.NoError(t, d.SetGroupPaused("test", true, "admin"))

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go w.Start(ctx)

	select {
	case <-executed:
		t.Fatal("task in paused group should not have run")
	case <-ctx.Done():
		// correct
	}
}

func TestWorker_SkipsPausedTask(t *testing.T) {
	executed := make(chan string, 1)
	exec := &mockExecutor{fn: func(task *models.Task, stdout, stderr io.Writer) error {
		executed <- task.Name
		return nil
	}}

	w, d := newTestWorker(t, exec)
	task := addTask(t, d, "paused-task")
	require.NoError(t, d.SetTaskPaused(task.Name, true))

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go w.Start(ctx)

	select {
	case <-executed:
		t.Fatal("paused task should not have run")
	case <-ctx.Done():
		// correct
	}
}

// ─── OutputCapture tests ─────────────────────────────────────────────────────

func TestOutputCapture_RingBuffer(t *testing.T) {
	oc := worker.NewOutputCapture(1, "stdout")
	for i := 0; i < 5; i++ {
		oc.Write([]byte("line\n"))
	}
	lines := oc.Lines()
	require.Len(t, lines, 5)
	for _, l := range lines {
		require.Equal(t, "line", l.Line)
	}
}

func TestOutputCapture_Subscribe(t *testing.T) {
	oc := worker.NewOutputCapture(2, "stdout")
	ch := oc.Subscribe()

	go func() {
		time.Sleep(10 * time.Millisecond)
		oc.Write([]byte("hello\n"))
		oc.MarkDone()
	}()

	var received []worker.OutputLine
	for line := range ch {
		received = append(received, line)
	}
	require.Len(t, received, 1)
	require.Equal(t, "hello", received[0].Line)
}

func TestOutputCapture_MarkDone(t *testing.T) {
	oc := worker.NewOutputCapture(3, "stderr")
	require.False(t, oc.Done())
	oc.MarkDone()
	require.True(t, oc.Done())
}

func TestOutputRegistry_RegisterGet(t *testing.T) {
	const execID = int64(99901)
	stdout, stderr := worker.Registry.Register(execID)
	require.NotNil(t, stdout)
	require.NotNil(t, stderr)

	so, se, ok := worker.Registry.Get(execID)
	require.True(t, ok)
	require.NotNil(t, so)
	require.NotNil(t, se)

	worker.Registry.Unregister(execID)
	_, _, ok = worker.Registry.Get(execID)
	require.False(t, ok)
}

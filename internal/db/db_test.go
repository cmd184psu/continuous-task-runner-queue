package db_test

import (
	"testing"
	"time"

	"github.com/cmd184psu/ctrq/internal/db"
	"github.com/cmd184psu/ctrq/internal/models"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func seedGroup(t *testing.T, d *db.DB, name string, poolLimit int) {
	t.Helper()
	err := d.UpsertGroup(&models.Group{Name: name, PoolLimit: poolLimit, AllowedTypes: []string{}})
	require.NoError(t, err)
}

func seedTask(t *testing.T, d *db.DB, name, group, taskType string) *models.Task {
	t.Helper()
	task := &models.Task{
		Name: name, GroupName: group, TaskType: taskType,
		Enabled: true, Priority: 50, Args: `{"shell":"echo test"}`,
	}
	id, err := d.AddTask(task)
	require.NoError(t, err)
	task.ID = id
	return task
}

// ─── Group tests ─────────────────────────────────────────────────────────────

func TestUpsertGroup_CreateAndUpdate(t *testing.T) {
	d := newTestDB(t)
	err := d.UpsertGroup(&models.Group{Name: "batch", PoolLimit: 5, AllowedTypes: []string{"shell"}})
	require.NoError(t, err)

	g, err := d.GetGroup("batch")
	require.NoError(t, err)
	require.NotNil(t, g)
	require.Equal(t, 5, g.PoolLimit)
	require.Equal(t, []string{"shell"}, g.AllowedTypes)

	// update
	err = d.UpsertGroup(&models.Group{Name: "batch", PoolLimit: 10, AllowedTypes: []string{}})
	require.NoError(t, err)
	g, _ = d.GetGroup("batch")
	require.Equal(t, 10, g.PoolLimit)
}

func TestGetGroup_NotFound(t *testing.T) {
	d := newTestDB(t)
	g, err := d.GetGroup("nonexistent")
	require.NoError(t, err)
	require.Nil(t, g)
}

func TestListGroups(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "alpha", 2)
	seedGroup(t, d, "beta", 3)

	groups, err := d.ListGroups()
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "alpha", groups[0].Name)
	require.Equal(t, "beta", groups[1].Name)
}

func TestDeleteGroup_NoTasks(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "empty", 1)
	require.NoError(t, d.DeleteGroup("empty"))
	g, _ := d.GetGroup("empty")
	require.Nil(t, g)
}

func TestDeleteGroup_WithTasks(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "used", 1)
	seedTask(t, d, "t1", "used", "shell")
	err := d.DeleteGroup("used")
	require.Error(t, err)
}

func TestSetGroupPaused(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g1", 2)
	require.NoError(t, d.SetGroupPaused("g1", true, "admin"))

	g, _ := d.GetGroup("g1")
	require.True(t, g.Paused)
	require.Equal(t, "admin", g.PausedBy)
	require.NotNil(t, g.PausedAt)
}

// ─── Task tests ───────────────────────────────────────────────────────────────

func TestAddTask_Basic(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "bg", 2)
	task := &models.Task{
		Name: "my-task", GroupName: "bg", TaskType: "shell",
		Enabled: true, Priority: 10, Args: `{"shell":"ls"}`,
	}
	id, err := d.AddTask(task)
	require.NoError(t, err)
	require.Positive(t, id)

	got, err := d.GetTask("my-task")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "my-task", got.Name)
	require.Equal(t, 10, got.Priority)
	require.True(t, got.Enabled)
}

func TestAddTask_Upsert(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "bg", 2)
	task := &models.Task{Name: "t", GroupName: "bg", TaskType: "shell", Enabled: true, Priority: 50, Args: "{}"}
	id1, _ := d.AddTask(task)

	task.Priority = 99
	id2, err := d.AddTask(task)
	require.NoError(t, err)
	// upsert returns lastInsertId which may be 0 on update; check the value was updated
	_ = id1
	_ = id2
	got, _ := d.GetTask("t")
	require.Equal(t, 99, got.Priority)
}

func TestGetTask_NotFound(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetTask("nonexistent")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestListTasks_GroupFilter(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g1", 1)
	seedGroup(t, d, "g2", 1)
	seedTask(t, d, "t-g1", "g1", "shell")
	seedTask(t, d, "t-g2", "g2", "shell")

	all, _ := d.ListTasks("")
	require.Len(t, all, 2)

	g1tasks, _ := d.ListTasks("g1")
	require.Len(t, g1tasks, 1)
	require.Equal(t, "t-g1", g1tasks[0].Name)
}

func TestDeleteTask_CascadesExecutions(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 1)
	task := seedTask(t, d, "mytask", "g", "shell")

	execID, err := d.CreateExecution(task.ID, "worker1", time.Now())
	require.NoError(t, err)
	require.Positive(t, execID)

	require.NoError(t, d.DeleteTask("mytask"))

	// execution should be gone via CASCADE
	exec, err := d.GetExecution(execID)
	require.NoError(t, err)
	require.Nil(t, exec)
}

func TestSetTaskPaused(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 1)
	seedTask(t, d, "t", "g", "shell")

	require.NoError(t, d.SetTaskPaused("t", true))
	got, _ := d.GetTask("t")
	require.True(t, got.Paused)

	require.NoError(t, d.SetTaskPaused("t", false))
	got, _ = d.GetTask("t")
	require.False(t, got.Paused)
}

// ─── Eligible tasks ───────────────────────────────────────────────────────────

func TestGetEligibleTasks_NewTask(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	seedTask(t, d, "new-task", "g", "shell")

	tasks, err := d.GetEligibleTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
}

func TestGetEligibleTasks_Priority(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 5)
	t1 := &models.Task{Name: "high", GroupName: "g", TaskType: "shell", Enabled: true, Priority: 10, Args: "{}"}
	t2 := &models.Task{Name: "low", GroupName: "g", TaskType: "shell", Enabled: true, Priority: 90, Args: "{}"}
	d.AddTask(t1)
	d.AddTask(t2)

	tasks, _ := d.GetEligibleTasks()
	require.Len(t, tasks, 2)
	require.Equal(t, "high", tasks[0].Name)
}

func TestGetEligibleTasks_Cooldown(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := &models.Task{Name: "cooldown-task", GroupName: "g", TaskType: "shell", Enabled: true,
		Priority: 50, Args: "{}", Repeat: true, CooldownSeconds: 3600}
	id, _ := d.AddTask(task)

	execID, _ := d.CreateExecution(id, "w", time.Now())
	require.NoError(t, d.FinishExecution(execID, "success", nil, 100, 0))

	tasks, _ := d.GetEligibleTasks()
	require.Empty(t, tasks, "task in cooldown should not be eligible")
}

func TestGetEligibleTasks_GroupPaused(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	seedTask(t, d, "t", "g", "shell")
	require.NoError(t, d.SetGroupPaused("g", true, "admin"))

	tasks, _ := d.GetEligibleTasks()
	require.Empty(t, tasks)
}

func TestGetEligibleTasks_TaskPaused(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	seedTask(t, d, "t", "g", "shell")
	require.NoError(t, d.SetTaskPaused("t", true))

	tasks, _ := d.GetEligibleTasks()
	require.Empty(t, tasks)
}

func TestGetEligibleTasks_Locked(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "locked-task", "g", "shell")

	ok, err := d.AcquireLock(task.ID, "worker1", 10*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	tasks, _ := d.GetEligibleTasks()
	require.Empty(t, tasks)
}

func TestGetEligibleTasks_PendingEnqueue(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := &models.Task{Name: "once", GroupName: "g", TaskType: "shell", Enabled: true,
		Priority: 50, Args: "{}", Repeat: false}
	id, _ := d.AddTask(task)

	// Mark as already completed once
	execID, _ := d.CreateExecution(id, "w", time.Now())
	require.NoError(t, d.FinishExecution(execID, "success", nil, 50, 0))

	// Without repeat and no pending, not eligible
	tasks, _ := d.GetEligibleTasks()
	require.Empty(t, tasks)

	// Force enqueue
	_, err := d.EnqueueTask("once", time.Now())
	require.NoError(t, err)

	tasks, _ = d.GetEligibleTasks()
	require.Len(t, tasks, 1)
}

// ─── Locking tests ────────────────────────────────────────────────────────────

func TestAcquireLock_Exclusive(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "t", "g", "shell")

	ok1, err := d.AcquireLock(task.ID, "worker1", time.Minute)
	require.NoError(t, err)
	require.True(t, ok1)

	ok2, err := d.AcquireLock(task.ID, "worker2", time.Minute)
	require.NoError(t, err)
	require.False(t, ok2)
}

func TestAcquireLock_AfterExpiry(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "t", "g", "shell")

	ok, _ := d.AcquireLock(task.ID, "worker1", -time.Second) // already expired
	require.True(t, ok)

	require.NoError(t, d.CleanupExpiredLocks())

	ok2, _ := d.AcquireLock(task.ID, "worker2", time.Minute)
	require.True(t, ok2)
}

func TestReleaseLock(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "t", "g", "shell")

	d.AcquireLock(task.ID, "w1", time.Minute)
	require.NoError(t, d.ReleaseLock(task.ID, "w1"))

	ok, _ := d.AcquireLock(task.ID, "w2", time.Minute)
	require.True(t, ok)
}

// ─── Execution tests ──────────────────────────────────────────────────────────

func TestCreateExecution_FinishExecution(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "t", "g", "shell")

	scheduled := time.Now().Add(-500 * time.Millisecond)
	execID, err := d.CreateExecution(task.ID, "worker1", scheduled)
	require.NoError(t, err)
	require.Positive(t, execID)

	require.NoError(t, d.StartExecution(execID, "worker1"))

	errMsg := "something failed"
	require.NoError(t, d.FinishExecution(execID, "failed", &errMsg, 1234, 55))

	exec, err := d.GetExecution(execID)
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.Equal(t, "failed", exec.Status)
	require.NotNil(t, exec.ErrorMessage)
	require.Equal(t, "something failed", *exec.ErrorMessage)
	require.NotNil(t, exec.DurationMs)
	require.Equal(t, int64(1234), *exec.DurationMs)
}

func TestListExecutions(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "t", "g", "shell")

	for i := 0; i < 3; i++ {
		id, _ := d.CreateExecution(task.ID, "w", time.Now())
		d.FinishExecution(id, "success", nil, 100, 0)
	}

	execs, err := d.ListExecutions("", 10)
	require.NoError(t, err)
	require.Len(t, execs, 3)
}

// ─── Metrics tests ────────────────────────────────────────────────────────────

func TestRecordMetric_GetMetrics(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 2)
	task := seedTask(t, d, "t", "g", "shell")

	execID, _ := d.CreateExecution(task.ID, "w", time.Now())
	require.NoError(t, d.RecordMetric(task.ID, execID, "success", 500, 10))
	require.NoError(t, d.RecordMetric(task.ID, execID, "failed", 200, 5))

	summaries, err := d.GetMetrics("", "", 24)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	s := summaries[0]
	require.Equal(t, "t", s.TaskName)
	require.Equal(t, 1, s.SuccessCount)
	require.Equal(t, 1, s.FailedCount)
	require.NotNil(t, s.AvgDurationMs)
}

func TestGetMetrics_GroupFilter(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g1", 1)
	seedGroup(t, d, "g2", 1)
	task1 := seedTask(t, d, "t1", "g1", "shell")
	task2 := seedTask(t, d, "t2", "g2", "shell")

	e1, _ := d.CreateExecution(task1.ID, "w", time.Now())
	d.RecordMetric(task1.ID, e1, "success", 100, 0)
	e2, _ := d.CreateExecution(task2.ID, "w", time.Now())
	d.RecordMetric(task2.ID, e2, "success", 200, 0)

	summaries, _ := d.GetMetrics("g1", "", 24)
	require.Len(t, summaries, 1)
	require.Equal(t, "t1", summaries[0].TaskName)
}

func TestCountRunningInGroup(t *testing.T) {
	d := newTestDB(t)
	seedGroup(t, d, "g", 5)
	task := seedTask(t, d, "t", "g", "shell")

	count, err := d.CountRunningInGroup("g")
	require.NoError(t, err)
	require.Equal(t, 0, count)

	execID, _ := d.CreateExecution(task.ID, "w", time.Now())
	d.StartExecution(execID, "w")

	count, err = d.CountRunningInGroup("g")
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

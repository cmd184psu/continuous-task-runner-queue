package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cmd184psu/ctrq/internal/models"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_journal_mode=WAL"
	}
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)
	// Enable foreign keys for every connection; must run before any DML.
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Ping() error {
	return db.conn.Ping()
}

func (db *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);

CREATE TABLE IF NOT EXISTS groups (
  name TEXT PRIMARY KEY,
  pool_limit INTEGER NOT NULL DEFAULT 1,
  allowed_types TEXT NOT NULL DEFAULT '[]',
  paused INTEGER NOT NULL DEFAULT 0,
  paused_at INTEGER,
  paused_by TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT UNIQUE NOT NULL,
  group_name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  paused INTEGER NOT NULL DEFAULT 0,
  priority INTEGER NOT NULL DEFAULT 50,
  cooldown_seconds INTEGER NOT NULL DEFAULT 0,
  repeat INTEGER NOT NULL DEFAULT 0,
  task_type TEXT NOT NULL,
  args TEXT NOT NULL DEFAULT '{}',
  sudo INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS task_executions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  scheduled_at INTEGER,
  started_at INTEGER,
  finished_at INTEGER,
  status TEXT NOT NULL DEFAULT 'pending',
  error_message TEXT,
  worker_id TEXT,
  duration_ms INTEGER,
  schedule_delay_ms INTEGER
);

CREATE TABLE IF NOT EXISTS task_locks (
  task_id INTEGER PRIMARY KEY,
  worker_id TEXT NOT NULL,
  acquired_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS task_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL,
  execution_id INTEGER NOT NULL,
  recorded_at INTEGER NOT NULL,
  duration_ms INTEGER,
  schedule_delay_ms INTEGER,
  status TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_group ON tasks(group_name);
CREATE INDEX IF NOT EXISTS idx_tasks_enabled ON tasks(enabled, priority);
CREATE INDEX IF NOT EXISTS idx_executions_task ON task_executions(task_id, status);
CREATE INDEX IF NOT EXISTS idx_executions_finished ON task_executions(finished_at);
CREATE INDEX IF NOT EXISTS idx_locks_expires ON task_locks(expires_at);
CREATE INDEX IF NOT EXISTS idx_metrics_task ON task_metrics(task_id, recorded_at);
`
	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}
	return db.applyMigrations()
}

func (db *DB) applyMigrations() error {
	var version int
	_ = db.conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version)

	migrations := []struct {
		version int
		sql     string
	}{
		{1, `ALTER TABLE tasks ADD COLUMN output_file TEXT NOT NULL DEFAULT ''`},
	}

	for _, m := range migrations {
		if m.version <= version {
			continue
		}
		if _, err := db.conn.Exec(m.sql); err != nil {
			return fmt.Errorf("migration %d: %w", m.version, err)
		}
		if _, err := db.conn.Exec(`INSERT OR REPLACE INTO schema_version (version) VALUES (?)`, m.version); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}
	return nil
}

// ─── Group CRUD ──────────────────────────────────────────────────────────────

func (db *DB) UpsertGroup(g *models.Group) error {
	typesJSON, err := json.Marshal(g.AllowedTypes)
	if err != nil {
		return err
	}
	now := epochMs(time.Now())
	_, err = db.conn.Exec(`
		INSERT INTO groups (name, pool_limit, allowed_types, paused, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
		  pool_limit = excluded.pool_limit,
		  allowed_types = excluded.allowed_types,
		  updated_at = excluded.updated_at`,
		g.Name, g.PoolLimit, string(typesJSON), now, now)
	return err
}

func (db *DB) GetGroup(name string) (*models.Group, error) {
	row := db.conn.QueryRow(`SELECT name, pool_limit, allowed_types, paused, paused_at, paused_by, created_at, updated_at FROM groups WHERE name = ?`, name)
	g, err := scanGroup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return g, err
}

func (db *DB) ListGroups() ([]*models.Group, error) {
	rows, err := db.conn.Query(`SELECT name, pool_limit, allowed_types, paused, paused_at, paused_by, created_at, updated_at FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []*models.Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (db *DB) DeleteGroup(name string) error {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM tasks WHERE group_name = ?`, name).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("group %q has %d task(s); delete or move them first", name, count)
	}
	_, err = db.conn.Exec(`DELETE FROM groups WHERE name = ?`, name)
	return err
}

func (db *DB) SetGroupPaused(name string, paused bool, by string) error {
	var pausedAt any
	if paused {
		pausedAt = epochMs(time.Now())
	}
	pausedInt := 0
	if paused {
		pausedInt = 1
	}
	_, err := db.conn.Exec(`UPDATE groups SET paused = ?, paused_at = ?, paused_by = ?, updated_at = ? WHERE name = ?`,
		pausedInt, pausedAt, by, epochMs(time.Now()), name)
	return err
}

func (db *DB) CountRunningInGroup(groupName string) (int, error) {
	var count int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM task_executions te
		JOIN tasks t ON t.id = te.task_id
		WHERE t.group_name = ? AND te.status = 'running'`, groupName).Scan(&count)
	return count, err
}

// ─── Task CRUD ────────────────────────────────────────────────────────────────

func (db *DB) AddTask(task *models.Task) (int64, error) {
	now := epochMs(time.Now())
	if task.Args == "" {
		task.Args = "{}"
	}
	result, err := db.conn.Exec(`
		INSERT INTO tasks (name, group_name, enabled, paused, priority, cooldown_seconds, repeat, task_type, args, sudo, output_file, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
		  group_name = excluded.group_name,
		  enabled = excluded.enabled,
		  paused = excluded.paused,
		  priority = excluded.priority,
		  cooldown_seconds = excluded.cooldown_seconds,
		  repeat = excluded.repeat,
		  task_type = excluded.task_type,
		  args = excluded.args,
		  sudo = excluded.sudo,
		  output_file = excluded.output_file,
		  updated_at = excluded.updated_at`,
		task.Name, task.GroupName, boolInt(task.Enabled), boolInt(task.Paused),
		task.Priority, task.CooldownSeconds, boolInt(task.Repeat),
		task.TaskType, task.Args, boolInt(task.Sudo), task.OutputFile, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) GetTask(name string) (*models.Task, error) {
	row := db.conn.QueryRow(`SELECT id, name, group_name, enabled, paused, priority, cooldown_seconds, repeat, task_type, args, sudo, output_file, created_at, updated_at FROM tasks WHERE name = ?`, name)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (db *DB) ListTasks(groupFilter string) ([]*models.Task, error) {
	q := `SELECT id, name, group_name, enabled, paused, priority, cooldown_seconds, repeat, task_type, args, sudo, output_file, created_at, updated_at FROM tasks`
	args := []any{}
	if groupFilter != "" {
		q += " WHERE group_name = ?"
		args = append(args, groupFilter)
	}
	q += " ORDER BY group_name, priority, name"
	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*models.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (db *DB) UpdateTask(name string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = epochMs(time.Now())
	setClauses := make([]string, 0, len(updates))
	vals := make([]any, 0, len(updates)+1)
	for k, v := range updates {
		setClauses = append(setClauses, k+" = ?")
		vals = append(vals, v)
	}
	vals = append(vals, name)
	q := "UPDATE tasks SET " + strings.Join(setClauses, ", ") + " WHERE name = ?"
	_, err := db.conn.Exec(q, vals...)
	return err
}

func (db *DB) DeleteTask(name string) error {
	_, err := db.conn.Exec(`DELETE FROM tasks WHERE name = ?`, name)
	return err
}

func (db *DB) SetTaskPaused(name string, paused bool) error {
	_, err := db.conn.Exec(`UPDATE tasks SET paused = ?, updated_at = ? WHERE name = ?`,
		boolInt(paused), epochMs(time.Now()), name)
	return err
}

// ─── Pool scheduling ─────────────────────────────────────────────────────────

func (db *DB) GetEligibleTasks() ([]*models.Task, error) {
	nowMs := epochMs(time.Now())
	q := `
WITH last_finished AS (
  SELECT task_id, MAX(finished_at) as last_fin
  FROM task_executions
  WHERE status IN ('success','failed')
  GROUP BY task_id
)
SELECT t.id, t.name, t.group_name, t.enabled, t.paused, t.priority,
       t.cooldown_seconds, t.repeat, t.task_type, t.args, t.sudo, t.output_file,
       t.created_at, t.updated_at
FROM tasks t
LEFT JOIN task_locks tl ON tl.task_id = t.id AND tl.expires_at > ?
LEFT JOIN last_finished lf ON lf.task_id = t.id
LEFT JOIN groups gs ON gs.name = t.group_name
WHERE t.enabled = 1
  AND t.paused = 0
  AND tl.task_id IS NULL
  AND COALESCE(gs.paused, 0) = 0
  AND (
    EXISTS (SELECT 1 FROM task_executions pe WHERE pe.task_id = t.id AND pe.status = 'pending')
    OR lf.last_fin IS NULL
    OR (t.repeat = 1 AND (? - CAST(lf.last_fin AS INTEGER)) >= t.cooldown_seconds * 1000)
  )
ORDER BY t.priority ASC, COALESCE(lf.last_fin, 0) ASC`
	rows, err := db.conn.Query(q, nowMs, nowMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*models.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ─── Execution lifecycle ─────────────────────────────────────────────────────

func (db *DB) EnqueueTask(taskName string, scheduledAt time.Time) (int64, error) {
	var taskID int64
	err := db.conn.QueryRow(`SELECT id FROM tasks WHERE name = ?`, taskName).Scan(&taskID)
	if err != nil {
		return 0, fmt.Errorf("task %q not found: %w", taskName, err)
	}
	result, err := db.conn.Exec(`
		INSERT INTO task_executions (task_id, scheduled_at, status)
		VALUES (?, ?, 'pending')`, taskID, epochMs(scheduledAt))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) GetPendingExecution(taskID int64) (*models.TaskExecution, error) {
	row := db.conn.QueryRow(`
		SELECT te.id, te.task_id, te.scheduled_at, te.started_at, te.finished_at,
		       te.status, te.error_message, te.worker_id, te.duration_ms, te.schedule_delay_ms,
		       t.name
		FROM task_executions te
		LEFT JOIN tasks t ON t.id = te.task_id
		WHERE te.task_id = ? AND te.status = 'pending' ORDER BY te.id ASC LIMIT 1`, taskID)
	exec, err := scanExecution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return exec, err
}

func (db *DB) CreateExecution(taskID int64, workerID string, scheduledAt time.Time) (int64, error) {
	result, err := db.conn.Exec(`
		INSERT INTO task_executions (task_id, scheduled_at, status, worker_id)
		VALUES (?, ?, 'pending', ?)`, taskID, epochMs(scheduledAt), workerID)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) StartExecution(execID int64, workerID string) error {
	now := epochMs(time.Now())
	_, err := db.conn.Exec(`
		UPDATE task_executions SET status = 'running', started_at = ?, worker_id = ?
		WHERE id = ?`, now, workerID, execID)
	return err
}

func (db *DB) FinishExecution(execID int64, status string, errMsg *string, durationMs, schedDelay int64) error {
	now := epochMs(time.Now())
	_, err := db.conn.Exec(`
		UPDATE task_executions SET status = ?, finished_at = ?, duration_ms = ?, schedule_delay_ms = ?, error_message = ?
		WHERE id = ?`, status, now, durationMs, schedDelay, errMsg, execID)
	return err
}

func (db *DB) GetExecution(id int64) (*models.TaskExecution, error) {
	row := db.conn.QueryRow(`
		SELECT te.id, te.task_id, te.scheduled_at, te.started_at, te.finished_at,
		       te.status, te.error_message, te.worker_id, te.duration_ms, te.schedule_delay_ms,
		       t.name
		FROM task_executions te
		LEFT JOIN tasks t ON t.id = te.task_id
		WHERE te.id = ?`, id)
	exec, err := scanExecution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return exec, err
}

func (db *DB) ListExecutions(taskFilter string, limit int) ([]*models.TaskExecution, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if taskFilter != "" {
		rows, err = db.conn.Query(`
			SELECT te.id, te.task_id, te.scheduled_at, te.started_at, te.finished_at,
			       te.status, te.error_message, te.worker_id, te.duration_ms, te.schedule_delay_ms,
			       t.name
			FROM task_executions te
			JOIN tasks t ON t.id = te.task_id
			WHERE t.name = ?
			ORDER BY te.id DESC LIMIT ?`, taskFilter, limit)
	} else {
		rows, err = db.conn.Query(`
			SELECT te.id, te.task_id, te.scheduled_at, te.started_at, te.finished_at,
			       te.status, te.error_message, te.worker_id, te.duration_ms, te.schedule_delay_ms,
			       t.name
			FROM task_executions te
			LEFT JOIN tasks t ON t.id = te.task_id
			ORDER BY te.id DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var execs []*models.TaskExecution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

// ─── Locking ──────────────────────────────────────────────────────────────────

func (db *DB) AcquireLock(taskID int64, workerID string, ttl time.Duration) (bool, error) {
	now := time.Now()
	nowMs := epochMs(now)
	expiresMs := epochMs(now.Add(ttl))
	result, err := db.conn.Exec(`
		INSERT INTO task_locks (task_id, worker_id, acquired_at, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(task_id) DO NOTHING`,
		taskID, workerID, nowMs, expiresMs)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func (db *DB) ReleaseLock(taskID int64, workerID string) error {
	_, err := db.conn.Exec(`DELETE FROM task_locks WHERE task_id = ? AND worker_id = ?`, taskID, workerID)
	return err
}

func (db *DB) CleanupExpiredLocks() error {
	_, err := db.conn.Exec(`DELETE FROM task_locks WHERE expires_at < ?`, epochMs(time.Now()))
	return err
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

func (db *DB) RecordMetric(taskID, execID int64, status string, durationMs, schedDelay int64) error {
	_, err := db.conn.Exec(`
		INSERT INTO task_metrics (task_id, execution_id, recorded_at, duration_ms, schedule_delay_ms, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		taskID, execID, epochMs(time.Now()), durationMs, schedDelay, status)
	return err
}

func (db *DB) GetMetrics(groupFilter, taskFilter string, hours int) ([]*models.MetricSummary, error) {
	if hours <= 0 {
		hours = 24
	}
	sinceMs := epochMs(time.Now().Add(-time.Duration(hours) * time.Hour))

	conditions := []string{"m.recorded_at >= ?"}
	args := []any{sinceMs}

	if groupFilter != "" {
		conditions = append(conditions, "t.group_name = ?")
		args = append(args, groupFilter)
	}
	if taskFilter != "" {
		conditions = append(conditions, "t.name = ?")
		args = append(args, taskFilter)
	}

	where := strings.Join(conditions, " AND ")
	q := fmt.Sprintf(`
		SELECT t.name, t.group_name,
		  SUM(CASE WHEN m.status = 'success' THEN 1 ELSE 0 END) as success_count,
		  SUM(CASE WHEN m.status = 'failed' THEN 1 ELSE 0 END) as failed_count,
		  AVG(m.duration_ms) as avg_duration_ms,
		  MIN(m.duration_ms) as min_duration_ms,
		  MAX(m.duration_ms) as max_duration_ms,
		  AVG(m.schedule_delay_ms) as avg_delay_ms,
		  MAX(m.recorded_at) as last_execution
		FROM task_metrics m
		JOIN tasks t ON t.id = m.task_id
		WHERE %s
		GROUP BY t.id, t.name, t.group_name
		ORDER BY t.group_name, t.name`, where)

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.MetricSummary
	for rows.Next() {
		var ms models.MetricSummary
		var lastExecMs *int64
		err := rows.Scan(&ms.TaskName, &ms.GroupName,
			&ms.SuccessCount, &ms.FailedCount,
			&ms.AvgDurationMs, &ms.MinDurationMs, &ms.MaxDurationMs,
			&ms.AvgDelayMs, &lastExecMs)
		if err != nil {
			return nil, err
		}
		if lastExecMs != nil {
			t := msToTime(*lastExecMs)
			ms.LastExecution = &t
		}
		results = append(results, &ms)
	}
	return results, rows.Err()
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanGroup(s scanner) (*models.Group, error) {
	var g models.Group
	var typesJSON string
	var pausedAt *int64
	var pausedBy *string
	var createdAt, updatedAt int64
	err := s.Scan(&g.Name, &g.PoolLimit, &typesJSON, &g.Paused, &pausedAt, &pausedBy, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(typesJSON), &g.AllowedTypes)
	if g.AllowedTypes == nil {
		g.AllowedTypes = []string{}
	}
	if pausedAt != nil {
		t := msToTime(*pausedAt)
		g.PausedAt = &t
	}
	if pausedBy != nil {
		g.PausedBy = *pausedBy
	}
	g.CreatedAt = msToTime(createdAt)
	g.UpdatedAt = msToTime(updatedAt)
	return &g, nil
}

func scanTask(s scanner) (*models.Task, error) {
	var t models.Task
	var createdAt, updatedAt int64
	var enabled, paused, repeat, sudo int
	err := s.Scan(&t.ID, &t.Name, &t.GroupName, &enabled, &paused, &t.Priority,
		&t.CooldownSeconds, &repeat, &t.TaskType, &t.Args, &sudo, &t.OutputFile, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	t.Paused = paused == 1
	t.Repeat = repeat == 1
	t.Sudo = sudo == 1
	t.CreatedAt = msToTime(createdAt)
	t.UpdatedAt = msToTime(updatedAt)
	return &t, nil
}

func scanExecution(s scanner) (*models.TaskExecution, error) {
	var e models.TaskExecution
	var scheduledAt, startedAt, finishedAt *int64
	var taskName *string
	err := s.Scan(&e.ID, &e.TaskID, &scheduledAt, &startedAt, &finishedAt,
		&e.Status, &e.ErrorMessage, &e.WorkerID, &e.DurationMs, &e.ScheduleDelayMs,
		&taskName)
	if err != nil {
		return nil, err
	}
	if taskName != nil {
		e.TaskName = *taskName
	}
	if scheduledAt != nil {
		t := msToTime(*scheduledAt)
		e.ScheduledAt = &t
	}
	if startedAt != nil {
		t := msToTime(*startedAt)
		e.StartedAt = &t
	}
	if finishedAt != nil {
		t := msToTime(*finishedAt)
		e.FinishedAt = &t
	}
	return &e, nil
}

func epochMs(t time.Time) int64 {
	return t.UnixMilli()
}

func msToTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

package models

import "time"

type GroupConfig struct {
	Name         string   `json:"name"`
	PoolLimit    int      `json:"pool_limit"`
	AllowedTypes []string `json:"allowed_types,omitempty"`
}

type Config struct {
	Port      int           `json:"port"`
	Passcode  string        `json:"passcode"`
	UIEnabled bool          `json:"ui_enabled"`
	DBPath    string        `json:"db_path"`
	Groups    []GroupConfig `json:"groups"`
}

type Task struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	GroupName       string    `json:"group_name"`
	Enabled         bool      `json:"enabled"`
	Paused          bool      `json:"paused"`
	Priority        int       `json:"priority"`
	CooldownSeconds int       `json:"cooldown_seconds"`
	Repeat          bool      `json:"repeat"`
	TaskType        string    `json:"task_type"`
	Args            string    `json:"args"`
	Sudo            bool      `json:"sudo"`
	OutputFile      string    `json:"output_file,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type TaskExecution struct {
	ID              int64      `json:"id"`
	TaskID          int64      `json:"task_id"`
	TaskName        string     `json:"task_name,omitempty"`
	ScheduledAt     *time.Time `json:"scheduled_at"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	Status          string     `json:"status"`
	ErrorMessage    *string    `json:"error_message"`
	WorkerID        *string    `json:"worker_id"`
	DurationMs      *int64     `json:"duration_ms"`
	ScheduleDelayMs *int64     `json:"schedule_delay_ms"`
}

type Group struct {
	Name         string     `json:"name"`
	PoolLimit    int        `json:"pool_limit"`
	AllowedTypes []string   `json:"allowed_types"`
	Paused       bool       `json:"paused"`
	PausedAt     *time.Time `json:"paused_at,omitempty"`
	PausedBy     string     `json:"paused_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type GroupStatus struct {
	Group
	RunningCount int `json:"running_count"`
}

type MetricSummary struct {
	TaskName      string     `json:"task_name"`
	GroupName     string     `json:"group_name"`
	SuccessCount  int        `json:"success_count"`
	FailedCount   int        `json:"failed_count"`
	AvgDurationMs *float64   `json:"avg_duration_ms"`
	MinDurationMs *int64     `json:"min_duration_ms"`
	MaxDurationMs *int64     `json:"max_duration_ms"`
	AvgDelayMs    *float64   `json:"avg_schedule_delay_ms"`
	LastExecution *time.Time `json:"last_execution"`
}

type AuthRequest struct {
	Passcode string `json:"passcode"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

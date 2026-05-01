package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"github.com/cmd184psu/ctrq/internal/models"
)

// Executor executes a task and writes output to the provided captures.
type Executor interface {
	Execute(task *models.Task, stdout, stderr io.Writer) error
}

type TaskExecutor struct{}

type execArgs struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Workdir string            `json:"workdir"`
	Env     map[string]string `json:"env"`
}

type shellArgs struct {
	Shell string `json:"shell"`
}

type scriptArgs struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
}

type migrationArgs struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

func (te *TaskExecutor) Execute(task *models.Task, stdout, stderr io.Writer) error {
	switch task.TaskType {
	case "exec":
		return te.executeExec(task, stdout, stderr)
	case "shell":
		return te.executeShell(task, stdout, stderr)
	case "script":
		return te.executeScript(task, stdout, stderr)
	case "migration":
		return te.executeMigration(task, stdout, stderr)
	default:
		return fmt.Errorf("unknown task_type: %q", task.TaskType)
	}
}

func (te *TaskExecutor) executeExec(task *models.Task, stdout, stderr io.Writer) error {
	var a execArgs
	if err := json.Unmarshal([]byte(task.Args), &a); err != nil {
		return fmt.Errorf("parse exec args: %w", err)
	}
	args := a.Args
	cmd := buildCmd(task.Sudo, a.Command, args...)
	if a.Workdir != "" {
		cmd.Dir = a.Workdir
	}
	for k, v := range a.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (te *TaskExecutor) executeShell(task *models.Task, stdout, stderr io.Writer) error {
	var a shellArgs
	if err := json.Unmarshal([]byte(task.Args), &a); err != nil {
		return fmt.Errorf(`parse shell args: %w — expected {"shell":"command"}`, err)
	}
	if a.Shell == "" {
		return fmt.Errorf(`shell args missing "shell" field — expected {"shell":"command"}`)
	}
	cmd := buildCmd(task.Sudo, "/bin/sh", "-c", a.Shell)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (te *TaskExecutor) executeScript(task *models.Task, stdout, stderr io.Writer) error {
	var a scriptArgs
	if err := json.Unmarshal([]byte(task.Args), &a); err != nil {
		return fmt.Errorf("parse script args: %w", err)
	}
	cmd := buildCmd(task.Sudo, a.Path, a.Args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (te *TaskExecutor) executeMigration(task *models.Task, stdout, stderr io.Writer) error {
	var a migrationArgs
	if err := json.Unmarshal([]byte(task.Args), &a); err != nil {
		return fmt.Errorf("parse migration args: %w", err)
	}
	migrateCmd := "migrate"
	if a.Command != "" {
		migrateCmd = a.Command
	}
	cmd := buildCmd(task.Sudo, migrateCmd, "--name", a.Name)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func buildCmd(sudo bool, name string, args ...string) *exec.Cmd {
	if sudo {
		allArgs := append([]string{name}, args...)
		return exec.Command("sudo", allArgs...)
	}
	return exec.Command(name, args...)
}

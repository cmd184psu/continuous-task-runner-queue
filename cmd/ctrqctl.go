package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cmd184psu/ctrq/internal/config"
	"github.com/cmd184psu/ctrq/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

var baseURL string

func RunCLI() {
	urlFlag := flag.String("url", "", "override server URL (default: from config)")
	flag.Parse()

	cfg, err := config.Load(config.DefaultConfigPath)
	if err != nil {
		fatalf("load config: %v", err)
	}

	baseURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	if *urlFlag != "" {
		baseURL = *urlFlag
	}
	if u := os.Getenv("CTRQ_URL"); u != "" {
		baseURL = u
	}

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	token, err := getToken(baseURL, cfg)
	if err != nil {
		fatalf("auth: %v", err)
	}

	noun := args[0]
	rest := args[1:]

	switch noun {
	case "group":
		runGroup(token, rest)
	case "task":
		runTask(token, rest)
	case "executions":
		runExecutions(token, rest)
	case "output":
		runOutput(token, rest)
	case "metrics":
		runMetrics(token, rest)
	case "health":
		runHealth()
	default:
		fmt.Fprintf(os.Stderr, "unknown noun: %s\n\n", noun)
		usage()
		os.Exit(1)
	}
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

func getToken(base string, cfg *models.Config) (string, error) {
	tokenPath := config.ExpandHome("~/.ctrq-token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		tok := strings.TrimSpace(string(data))
		p := jwt.NewParser(jwt.WithoutClaimsValidation())
		t, _, err := p.ParseUnverified(tok, &jwt.RegisteredClaims{})
		if err == nil {
			if claims, ok := t.Claims.(*jwt.RegisteredClaims); ok {
				if claims.ExpiresAt != nil && time.Until(claims.ExpiresAt.Time) > 5*time.Minute {
					return tok, nil
				}
			}
		}
	}
	body, _ := json.Marshal(models.AuthRequest{Passcode: cfg.Passcode})
	resp, err := http.Post(base+"/api/auth/token", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("connect to server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth failed (status %d) — check passcode in ~/.ctrq.json", resp.StatusCode)
	}
	var ar models.AuthResponse
	json.NewDecoder(resp.Body).Decode(&ar)
	os.WriteFile(tokenPath, []byte(ar.Token), 0600)
	return ar.Token, nil
}

// ─── Group commands ───────────────────────────────────────────────────────────

func runGroup(token string, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: ctrqctl group <list|pause|resume|create|update|delete> [name] [flags]")
		os.Exit(1)
	}
	verb := args[0]
	switch verb {
	case "list":
		var groups []models.GroupStatus
		apiGet(token, "/api/groups", &groups)
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tLIMIT\tRUNNING\tPAUSED")
		for _, g := range groups {
			paused := "no"
			if g.Paused {
				paused = "YES"
			}
			fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", g.Name, g.PoolLimit, g.RunningCount, paused)
		}
		tw.Flush()

	case "pause":
		if len(args) < 2 {
			fatalf("usage: ctrqctl group pause <name>")
		}
		apiPost(token, "/api/groups/"+args[1]+"/pause", nil, nil)
		fmt.Printf("group %q paused\n", args[1])

	case "resume":
		if len(args) < 2 {
			fatalf("usage: ctrqctl group resume <name>")
		}
		apiPost(token, "/api/groups/"+args[1]+"/resume", nil, nil)
		fmt.Printf("group %q resumed\n", args[1])

	case "create":
		fs := flag.NewFlagSet("group create", flag.ExitOnError)
		name := fs.String("name", "", "group name (required)")
		limit := fs.Int("limit", 1, "pool limit")
		fs.Parse(args[1:])
		if *name == "" {
			fatalf("--name is required")
		}
		var result models.Group
		apiPost(token, "/api/groups", map[string]any{"name": *name, "pool_limit": *limit, "allowed_types": []string{}}, &result)
		fmt.Printf("created group %q (pool_limit=%d)\n", result.Name, result.PoolLimit)

	case "update":
		if len(args) < 2 {
			fatalf("usage: ctrqctl group update <name> [--limit N]")
		}
		fs := flag.NewFlagSet("group update", flag.ExitOnError)
		limit := fs.Int("limit", 0, "new pool limit")
		fs.Parse(args[2:])
		updates := map[string]any{}
		if *limit > 0 {
			updates["pool_limit"] = *limit
		}
		apiPut(token, "/api/groups/"+args[1], updates, nil)
		fmt.Printf("group %q updated\n", args[1])

	case "delete":
		if len(args) < 2 {
			fatalf("usage: ctrqctl group delete <name>")
		}
		apiDelete(token, "/api/groups/"+args[1])
		fmt.Printf("group %q deleted\n", args[1])

	default:
		fatalf("unknown group verb: %s", verb)
	}
}

// ─── Task commands ────────────────────────────────────────────────────────────

func runTask(token string, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: ctrqctl task <list|add|update|pause|resume|delete|enqueue> [flags]")
		os.Exit(1)
	}
	verb := args[0]
	switch verb {
	case "list":
		fs := flag.NewFlagSet("task list", flag.ExitOnError)
		group := fs.String("group", "", "filter by group")
		fs.Parse(args[1:])
		path := "/api/tasks"
		if *group != "" {
			path += "?group=" + *group
		}
		var tasks []models.Task
		apiGet(token, path, &tasks)
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tGROUP\tTYPE\tENABLED\tPAUSED\tREPEAT\tCOOLDOWN")
		for _, t := range tasks {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%v\t%v\t%ds\n",
				t.Name, t.GroupName, t.TaskType, t.Enabled, t.Paused, t.Repeat, t.CooldownSeconds)
		}
		tw.Flush()

	case "add":
		fs := flag.NewFlagSet("task add", flag.ExitOnError)
		name := fs.String("name", "", "task name (required)")
		group := fs.String("group", "", "group name (required)")
		taskType := fs.String("type", "shell", "task type: exec|shell|script|migration")
		taskArgs := fs.String("args", "{}", "task args JSON")
		repeat := fs.Bool("repeat", false, "re-enqueue after cooldown")
		cooldown := fs.Int("cooldown", 0, "cooldown seconds between runs")
		priority := fs.Int("priority", 50, "priority (lower runs first)")
		sudo := fs.Bool("sudo", false, "run with sudo")
		enabled := fs.Bool("enabled", true, "enable task immediately")
		outputFile := fs.String("output-file", "", "append output to this file path ({task} and {exec_id} supported)")
		fs.Parse(args[1:])
		if *name == "" || *group == "" {
			fatalf("--name and --group are required")
		}
		body := map[string]any{
			"name": *name, "group_name": *group, "task_type": *taskType,
			"args": *taskArgs, "repeat": *repeat, "cooldown_seconds": *cooldown,
			"priority": *priority, "sudo": *sudo, "enabled": *enabled,
			"output_file": *outputFile,
		}
		var task models.Task
		apiPostStatus(token, "/api/tasks", body, &task, http.StatusCreated)
		fmt.Printf("added task %q in group %q\n", task.Name, task.GroupName)

	case "update":
		if len(args) < 2 {
			fatalf("usage: ctrqctl task update <name> [--priority N] [--cooldown N] [--enabled] [--repeat] [--args JSON]")
		}
		name := args[1]
		fs := flag.NewFlagSet("task update", flag.ExitOnError)
		priority := fs.Int("priority", -1, "new priority")
		cooldown := fs.Int("cooldown", -1, "new cooldown seconds")
		repeat := fs.Bool("repeat", false, "set repeat")
		noRepeat := fs.Bool("no-repeat", false, "unset repeat")
		enabled := fs.Bool("enabled", false, "enable task")
		disabled := fs.Bool("disabled", false, "disable task")
		taskArgs := fs.String("args", "", "new args JSON")
		outputFile := fs.String("output-file", "\x00", "output file path (set to empty string to clear)")
		fs.Parse(args[2:])
		updates := map[string]any{}
		if *priority >= 0 {
			updates["priority"] = *priority
		}
		if *cooldown >= 0 {
			updates["cooldown_seconds"] = *cooldown
		}
		if *repeat {
			updates["repeat"] = 1
		}
		if *noRepeat {
			updates["repeat"] = 0
		}
		if *enabled {
			updates["enabled"] = 1
		}
		if *disabled {
			updates["enabled"] = 0
		}
		if *taskArgs != "" {
			updates["args"] = *taskArgs
		}
		if *outputFile != "\x00" {
			updates["output_file"] = *outputFile
		}
		apiPut(token, "/api/tasks/"+name, updates, nil)
		fmt.Printf("task %q updated\n", name)

	case "pause":
		if len(args) < 2 {
			fatalf("usage: ctrqctl task pause <name>")
		}
		apiPost(token, "/api/tasks/"+args[1]+"/pause", nil, nil)
		fmt.Printf("task %q paused\n", args[1])

	case "resume":
		if len(args) < 2 {
			fatalf("usage: ctrqctl task resume <name>")
		}
		apiPost(token, "/api/tasks/"+args[1]+"/resume", nil, nil)
		fmt.Printf("task %q resumed\n", args[1])

	case "delete":
		if len(args) < 2 {
			fatalf("usage: ctrqctl task delete <name>")
		}
		apiDelete(token, "/api/tasks/"+args[1])
		fmt.Printf("task %q deleted\n", args[1])

	case "enqueue":
		if len(args) < 2 {
			fatalf("usage: ctrqctl task enqueue <name>")
		}
		var result map[string]int64
		apiPostStatus(token, "/api/tasks/"+args[1]+"/enqueue", nil, &result, http.StatusCreated)
		fmt.Printf("enqueued task %q → execution id %d\n", args[1], result["execution_id"])

	default:
		fatalf("unknown task verb: %s", verb)
	}
}

// ─── Other commands ───────────────────────────────────────────────────────────

func runExecutions(token string, args []string) {
	fs := flag.NewFlagSet("executions", flag.ExitOnError)
	task := fs.String("task", "", "filter by task name")
	limit := fs.Int("limit", 20, "number of results")
	fs.Parse(args)
	path := fmt.Sprintf("/api/executions?limit=%d", *limit)
	if *task != "" {
		path += "&task=" + url.QueryEscape(*task)
	}
	var execs []models.TaskExecution
	apiGet(token, path, &execs)
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTASK\tSTATUS\tDURATION\tSTARTED")
	for _, e := range execs {
		dur := "-"
		if e.DurationMs != nil {
			dur = fmt.Sprintf("%dms", *e.DurationMs)
		}
		started := "-"
		if e.StartedAt != nil {
			started = e.StartedAt.Format("2006-01-02 15:04:05")
		}
		name := strconv.FormatInt(e.TaskID, 10)
		if e.TaskName != "" {
			name = e.TaskName
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", e.ID, name, e.Status, dur, started)
	}
	tw.Flush()
}

func runOutput(token string, args []string) {
	if len(args) < 1 {
		fatalf("usage: ctrqctl output <execution-id|task-name>")
	}
	execID := args[0]
	// If the argument is not a number, treat it as a task name and resolve
	// the most recent execution ID for that task.
	if _, err := strconv.ParseInt(execID, 10, 64); err != nil {
		var execs []models.TaskExecution
		apiGet(token, "/api/executions?task="+url.QueryEscape(execID)+"&limit=1", &execs)
		if len(execs) == 0 {
			fatalf("no executions found for task %q", execID)
		}
		execID = strconv.FormatInt(execs[0].ID, 10)
	}
	req, err := http.NewRequest("GET", baseURL+"/api/executions/"+execID+"/output", nil)
	if err != nil {
		fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fatalf("server error %d: %s", resp.StatusCode, body)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event struct {
				Stream string `json:"stream"`
				Line   string `json:"line"`
			}
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				fmt.Printf("[%s] %s\n", event.Stream, event.Line)
			}
		}
		if line == "event: done" {
			return
		}
	}
}

func runMetrics(token string, args []string) {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	group := fs.String("group", "", "filter by group")
	task := fs.String("task", "", "filter by task")
	hours := fs.Int("hours", 24, "time window in hours")
	fs.Parse(args)
	path := fmt.Sprintf("/api/metrics?hours=%d", *hours)
	if *group != "" {
		path += "&group=" + url.QueryEscape(*group)
	}
	if *task != "" {
		path += "&task=" + url.QueryEscape(*task)
	}
	var summaries []models.MetricSummary
	apiGet(token, path, &summaries)
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TASK\tGROUP\tOK\tFAIL\tAVG(ms)\tMIN\tMAX\tLAST")
	for _, s := range summaries {
		avg, minD, maxD := "-", "-", "-"
		if s.AvgDurationMs != nil {
			avg = fmt.Sprintf("%.0f", *s.AvgDurationMs)
		}
		if s.MinDurationMs != nil {
			minD = strconv.FormatInt(*s.MinDurationMs, 10)
		}
		if s.MaxDurationMs != nil {
			maxD = strconv.FormatInt(*s.MaxDurationMs, 10)
		}
		last := "-"
		if s.LastExecution != nil {
			last = s.LastExecution.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
			s.TaskName, s.GroupName, s.SuccessCount, s.FailedCount, avg, minD, maxD, last)
	}
	tw.Flush()
}

func runHealth() {
	resp, err := http.Get(baseURL + "/api/health")
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		fmt.Println("ok")
	} else {
		fmt.Printf("unhealthy (%d): %s\n", resp.StatusCode, body)
		os.Exit(1)
	}
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func apiGet(token, path string, out any) {
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	checkStatus(resp, path)
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
}

func apiPost(token, path string, body, out any) {
	apiPostStatus(token, path, body, out, http.StatusOK)
}

func apiPostStatus(token, path string, body, out any, expectedStatus int) {
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest("POST", baseURL+path, r)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		b, _ := io.ReadAll(resp.Body)
		fatalf("POST %s returned %d: %s", path, resp.StatusCode, b)
	}
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
}

func apiPut(token, path string, body, out any) {
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest("PUT", baseURL+path, r)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("PUT %s: %v", path, err)
	}
	defer resp.Body.Close()
	checkStatus(resp, path)
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
}

func apiDelete(token, path string) {
	req, _ := http.NewRequest("DELETE", baseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatalf("DELETE %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		fatalf("DELETE %s returned %d: %s", path, resp.StatusCode, b)
	}
}

func checkStatus(resp *http.Response, path string) {
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		fatalf("%s returned %d: %s", path, resp.StatusCode, b)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, `ctrqctl — continuous task runner queue CLI

Usage: ctrqctl [--url URL] <noun> <verb> [flags]

Nouns:
  group  list|create|update|delete|pause|resume
  task   list|add|update|pause|resume|delete|enqueue
  executions  [--task NAME] [--limit N]
  output <execution-id|task-name>
  metrics  [--group G] [--task T] [--hours N]
  health

Global flags:
  --url URL       server base URL (default: from ~/.ctrq.json port)
  CTRQ_URL env    alternative to --url`)
}

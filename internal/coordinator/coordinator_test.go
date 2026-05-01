package coordinator_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cmd184psu/ctrq/internal/coordinator"
	"github.com/cmd184psu/ctrq/internal/db"
	"github.com/cmd184psu/ctrq/internal/models"
	"github.com/stretchr/testify/require"
)

// testFlusher wraps httptest.ResponseRecorder to implement http.Flusher.
type testFlusher struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (tf *testFlusher) Flush() { tf.flushed = true }

func newTestServer(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	cfg := &models.Config{
		Port:      9898,
		Passcode:  "12345",
		UIEnabled: false,
		Groups:    []models.GroupConfig{{Name: "test", PoolLimit: 2}},
	}
	// seed group into DB
	err = d.UpsertGroup(&models.Group{Name: "test", PoolLimit: 2, AllowedTypes: []string{}})
	require.NoError(t, err)

	c := coordinator.New(d, cfg)
	srv := httptest.NewServer(coordinator.Routes(c))
	t.Cleanup(func() {
		srv.Close()
		d.Close()
	})
	return srv, d
}

func getToken(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	body, _ := json.Marshal(models.AuthRequest{Passcode: "12345"})
	resp, err := http.Post(srv.URL+"/api/auth/token", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ar models.AuthResponse
	json.NewDecoder(resp.Body).Decode(&ar)
	require.NotEmpty(t, ar.Token)
	return ar.Token
}

func authReq(t *testing.T, method, url, token string, body any) *http.Request {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ─── Auth tests ───────────────────────────────────────────────────────────────

func TestHandleAuthToken_CorrectPasscode(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	require.NotEmpty(t, token)
	// JWT has 3 dot-separated parts
	require.Equal(t, 3, len(strings.Split(token, ".")))
}

func TestHandleAuthToken_WrongPasscode(t *testing.T) {
	srv, _ := newTestServer(t)
	body, _ := json.Marshal(models.AuthRequest{Passcode: "00000"})
	resp, err := http.Post(srv.URL+"/api/auth/token", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHandleAuthToken_NoBody(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/api/auth/token", "application/json", strings.NewReader("bad json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMiddleware_NoToken(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/tasks")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestMiddleware_InvalidToken(t *testing.T) {
	srv, _ := newTestServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/api/tasks", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ─── Group tests ─────────────────────────────────────────────────────────────

func TestHandleListGroups(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "GET", srv.URL+"/api/groups", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var groups []models.GroupStatus
	json.NewDecoder(resp.Body).Decode(&groups)
	require.Len(t, groups, 1)
	require.Equal(t, "test", groups[0].Name)
}

func TestHandleCreateGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "POST", srv.URL+"/api/groups", token, map[string]any{
		"name": "new-group", "pool_limit": 3,
	})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestHandlePauseGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "POST", srv.URL+"/api/groups/test/pause", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandleResumeGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "POST", srv.URL+"/api/groups/test/resume", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandleDeleteGroup_WithTasks(t *testing.T) {
	srv, d := newTestServer(t)
	token := getToken(t, srv)
	_, err := d.AddTask(&models.Task{Name: "t", GroupName: "test", TaskType: "shell", Enabled: true, Priority: 50, Args: "{}"})
	require.NoError(t, err)

	req := authReq(t, "DELETE", srv.URL+"/api/groups/test", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

// ─── Task tests ───────────────────────────────────────────────────────────────

func TestHandleAddTask_Valid(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "POST", srv.URL+"/api/tasks", token, map[string]any{
		"name":       "my-task",
		"group_name": "test",
		"task_type":  "shell",
		"args":       `{"shell":"echo hi"}`,
		"enabled":    true,
	})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestHandleAddTask_UnknownGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "POST", srv.URL+"/api/tasks", token, map[string]any{
		"name": "bad", "group_name": "no-such-group", "task_type": "shell",
	})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleDeleteTask(t *testing.T) {
	srv, d := newTestServer(t)
	token := getToken(t, srv)
	_, err := d.AddTask(&models.Task{Name: "del-me", GroupName: "test", TaskType: "shell", Enabled: true, Priority: 50, Args: "{}"})
	require.NoError(t, err)

	req := authReq(t, "DELETE", srv.URL+"/api/tasks/del-me", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestHandleEnqueueTask(t *testing.T) {
	srv, d := newTestServer(t)
	token := getToken(t, srv)
	_, err := d.AddTask(&models.Task{Name: "enq", GroupName: "test", TaskType: "shell", Enabled: true, Priority: 50, Args: "{}"})
	require.NoError(t, err)

	req := authReq(t, "POST", srv.URL+"/api/tasks/enq/enqueue", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var result map[string]int64
	json.NewDecoder(resp.Body).Decode(&result)
	require.Positive(t, result["execution_id"])
}

func TestHandlePauseResumeTask(t *testing.T) {
	srv, d := newTestServer(t)
	token := getToken(t, srv)
	_, err := d.AddTask(&models.Task{Name: "pausable", GroupName: "test", TaskType: "shell", Enabled: true, Priority: 50, Args: "{}"})
	require.NoError(t, err)

	req := authReq(t, "POST", srv.URL+"/api/tasks/pausable/pause", token, nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	task, _ := d.GetTask("pausable")
	require.True(t, task.Paused)

	req = authReq(t, "POST", srv.URL+"/api/tasks/pausable/resume", token, nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	task, _ = d.GetTask("pausable")
	require.False(t, task.Paused)
}

// ─── Metrics/Executions ───────────────────────────────────────────────────────

func TestHandleMetrics(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "GET", srv.URL+"/api/metrics", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandleListExecutions(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "GET", srv.URL+"/api/executions", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandleHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/api/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── SSE output test ──────────────────────────────────────────────────────────

func TestHandleExecutionOutput_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getToken(t, srv)
	req := authReq(t, "GET", srv.URL+"/api/executions/99999/output", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// not in registry, not in DB → 404
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHandleExecutionOutput_CompletedExecution(t *testing.T) {
	srv, d := newTestServer(t)
	token := getToken(t, srv)

	task := &models.Task{Name: "t", GroupName: "test", TaskType: "shell", Enabled: true, Priority: 50, Args: "{}"}
	id, _ := d.AddTask(task)
	execID, _ := d.CreateExecution(id, "w", time.Now())
	d.StartExecution(execID, "w")
	d.FinishExecution(execID, "success", nil, 100, 0)

	req := authReq(t, "GET", srv.URL+"/api/executions/"+itoa(execID)+"/output", token, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// exec is in DB but not in registry → returns status event
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "success")
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

package coordinator

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cmd184psu/ctrq/internal/models"
)

func (c *Coordinator) handleListTasks(w http.ResponseWriter, r *http.Request) {
	groupFilter := r.URL.Query().Get("group")
	tasks, err := c.db.ListTasks(groupFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tasks == nil {
		tasks = []*models.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (c *Coordinator) handleAddTask(w http.ResponseWriter, r *http.Request) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if task.Name == "" || task.GroupName == "" || task.TaskType == "" {
		writeError(w, http.StatusBadRequest, "name, group_name, and task_type are required")
		return
	}
	if task.Args == "" {
		task.Args = "{}"
	}
	if !json.Valid([]byte(task.Args)) {
		writeError(w, http.StatusBadRequest, "args must be a valid JSON object")
		return
	}
	if task.Priority == 0 {
		task.Priority = 50
	}

	// validate group exists
	g, err := c.db.GetGroup(task.GroupName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if g == nil {
		writeError(w, http.StatusBadRequest, "unknown group: "+task.GroupName)
		return
	}

	_, err = c.db.AddTask(&task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	created, _ := c.db.GetTask(task.Name)
	writeJSON(w, http.StatusCreated, created)
}

func (c *Coordinator) handleGetTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	task, err := c.db.GetTask(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (c *Coordinator) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	existing, err := c.db.GetTask(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Strip read-only fields
	delete(updates, "id")
	delete(updates, "name")
	delete(updates, "created_at")

	if argsVal, ok := updates["args"]; ok {
		argsStr, _ := argsVal.(string)
		if argsStr == "" {
			updates["args"] = "{}"
		} else if !json.Valid([]byte(argsStr)) {
			writeError(w, http.StatusBadRequest, "args must be a valid JSON object")
			return
		}
	}

	if err := c.db.UpdateTask(name, updates); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, _ := c.db.GetTask(name)
	writeJSON(w, http.StatusOK, updated)
}

func (c *Coordinator) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := c.db.DeleteTask(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Coordinator) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := c.db.SetTaskPaused(name, true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (c *Coordinator) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := c.db.SetTaskPaused(name, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (c *Coordinator) handleEnqueueTask(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	execID, err := c.db.EnqueueTask(name, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"execution_id": execID})
}

package coordinator

import (
	"net/http"
	"strconv"

	"github.com/cmd184psu/ctrq/internal/models"
)

func (c *Coordinator) handleMetrics(w http.ResponseWriter, r *http.Request) {
	groupFilter := r.URL.Query().Get("group")
	taskFilter := r.URL.Query().Get("task")
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			hours = n
		}
	}
	summaries, err := c.db.GetMetrics(groupFilter, taskFilter, hours)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if summaries == nil {
		summaries = []*models.MetricSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

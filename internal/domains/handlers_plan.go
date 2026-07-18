// Domain plan assignment and resource-limit reapplication.
package domains

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"servika/internal/httpx"
	"servika/internal/resourcelimit"

	"github.com/go-chi/chi/v5"
)

// PUT /domains/{id}/plan  body: {"plan_id": 3}  (null removes the plan)
type setPlanReq struct {
	PlanID *int64 `json:"plan_id"`
}

func (h *Handlers) SetPlan(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setPlanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	// verify the plan exists
	if req.PlanID != nil {
		var n int
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM service_plans WHERE id=?`, *req.PlanID).Scan(&n); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Database operation failed")
			return
		}
		if n == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "Plan not found")
			return
		}
	}
	// Verify that the domain exists.
	var systemUser string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user FROM domains WHERE id=?`, id).Scan(&systemUser); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "Domain not found")
		} else {
			httpx.WriteError(w, http.StatusInternalServerError, "Database operation failed")
		}
		return
	}
	// update
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET plan_id=? WHERE id=?`, req.PlanID, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Plan assignment failed")
		return
	}
	// Reapply resource limits in the background with an independent context.
	// The request context is cancelled when the HTTP request ends and would interrupt the cgroup write.
	go func(did int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := resourcelimit.ApplyAll(ctx, h.DB, did); err != nil {
			log.Printf("resource limit apply domain=%d: %v", did, err)
		}
	}(id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "plan_id": req.PlanID})
}

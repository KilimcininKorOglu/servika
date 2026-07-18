// Package accounts provides customer account CRUD handlers.
package accounts

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Customer describes a customer account.
type Customer struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	PlanID  *int64 `json:"plan_id"`
	Status  string `json:"status"`
	Notes   string `json:"notes"`
	Created string `json:"created_at"`
}

// Handlers provides customer account HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

// ListCustomers returns all customer accounts.
func (h *Handlers) ListCustomers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, name, email, plan_id, status, notes, DATE_FORMAT(created_at,'%Y-%m-%d')
		 FROM customers ORDER BY id`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "customers could not be listed")
		return
	}
	defer rows.Close()
	out := make([]Customer, 0)
	for rows.Next() {
		var cs Customer
		if err := rows.Scan(&cs.ID, &cs.Name, &cs.Email, &cs.PlanID, &cs.Status, &cs.Notes, &cs.Created); err == nil {
			out = append(out, cs)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// CreateCustomer creates a customer account.
func (h *Handlers) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	var cs Customer
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if cs.Name == "" || cs.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name and email are required")
		return
	}
	if cs.Status == "" {
		cs.Status = "active"
	}
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO customers(name, email, plan_id, status, notes) VALUES(?,?,?,?,?)`,
		cs.Name, cs.Email, cs.PlanID, cs.Status, cs.Notes)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "customer could not be created")
		return
	}
	cs.ID, _ = res.LastInsertId()
	httpx.WriteJSON(w, http.StatusCreated, cs)
}

// UpdateCustomer updates a customer account.
func (h *Handlers) UpdateCustomer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var cs Customer
	if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE customers SET name=?, email=?, plan_id=?, status=?, notes=? WHERE id=?`,
		cs.Name, cs.Email, cs.PlanID, cs.Status, cs.Notes, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "customer could not be updated")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DeleteCustomer deletes a customer account without assigned domains.
func (h *Handlers) DeleteCustomer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var n int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains WHERE customer_id=?`, id).Scan(&n); err == nil && n > 0 {
		httpx.WriteError(w, http.StatusConflict, "remove this customer's domains first")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM customers WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "customer could not be deleted")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

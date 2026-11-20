package agent

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

type API struct{ Store *Store }

func NewHandler(store *Store) http.Handler {
	api := &API{Store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, map[string]string{"status": "ok", "mode": "deterministic-demo"})
	})
	mux.HandleFunc("GET /api/orders", api.listOrders)
	mux.HandleFunc("GET /api/orders/{id}", api.getOrder)
	mux.HandleFunc("POST /api/actions/preview", api.preview)
	mux.HandleFunc("POST /api/actions/execute", api.execute)
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Demo-Tenant")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	t := r.Header.Get("X-Demo-Tenant")
	if !ValidTenant(t) {
		problem(w, http.StatusUnauthorized, "Select a valid demo tenant; this header is not real authentication.")
		return "", false
	}
	return t, true
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func problem(w http.ResponseWriter, status int, reason string) {
	respond(w, status, map[string]string{"error": reason})
}
func storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		problem(w, http.StatusNotFound, "Order not found")
	case errors.Is(err, ErrConflict), errors.Is(err, ErrKeyConflict):
		problem(w, http.StatusConflict, err.Error())
	default:
		log.Printf("unexpected error: %v", err)
		problem(w, http.StatusInternalServerError, "Internal error")
	}
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		problem(w, http.StatusBadRequest, "Invalid JSON request")
		return false
	}
	return true
}
func (a *API) listOrders(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant(w, r)
	if !ok {
		return
	}
	respond(w, http.StatusOK, map[string]any{"orders": a.Store.List(t)})
}
func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	o, err := a.Store.Get(t, id)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, http.StatusOK, o)
}

type actionRequest struct {
	OrderID        string `json:"order_id"`
	Action         string `json:"action"`
	Confirmed      bool   `json:"confirmed"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant(w, r)
	if !ok {
		return
	}
	var request actionRequest
	if !decode(w, r, &request) {
		return
	}
	if strings.ToLower(request.Action) != "cancel" || request.OrderID == "" {
		problem(w, http.StatusBadRequest, "Expected cancel action and order_id")
		return
	}
	p, err := a.Store.PreviewCancel(t, request.OrderID)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, http.StatusOK, p)
}
func (a *API) execute(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant(w, r)
	if !ok {
		return
	}
	var request actionRequest
	if !decode(w, r, &request) {
		return
	}
	if strings.ToLower(request.Action) != "cancel" || request.OrderID == "" {
		problem(w, http.StatusBadRequest, "Expected cancel action and order_id")
		return
	}
	if !request.Confirmed {
		problem(w, http.StatusForbidden, "Explicit confirmation required")
		return
	}
	if !ValidKey(request.IdempotencyKey) {
		problem(w, http.StatusBadRequest, "Invalid idempotency key")
		return
	}
	receipt, err := a.Store.ExecuteCancel(t, request.OrderID, request.IdempotencyKey)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, http.StatusOK, receipt)
}

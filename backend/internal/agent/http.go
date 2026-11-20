package agent

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strings"
)

const maxBodyBytes = 8 * 1024

type API struct{ Store *Store }

func NewHandler(store *Store) http.Handler {
	api := &API{Store: store}
	mux := http.NewServeMux()
	// route serves one method on a path and answers every other method with a
	// JSON 405, so clients always get a JSON body they can parse.
	route := func(method, path string, h http.HandlerFunc) {
		mux.HandleFunc(method+" "+path, h)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", method)
			problem(w, http.StatusMethodNotAllowed, "Method not allowed")
		})
	}
	route(http.MethodGet, "/api/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, map[string]string{"status": "ok", "mode": "deterministic-demo"})
	})
	route(http.MethodGet, "/api/orders", api.listOrders)
	route(http.MethodGet, "/api/orders/{id}", api.getOrder)
	route(http.MethodPost, "/api/actions/preview", api.preview)
	route(http.MethodPost, "/api/actions/execute", api.execute)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		problem(w, http.StatusNotFound, "Not found")
	})
	return cors(mux)
}

// Browser origins of the React UI: the Vite dev server and `vite preview`.
var allowedOrigins = map[string]bool{
	"http://localhost:5173": true,
	"http://127.0.0.1:5173": true,
	"http://localhost:4173": true,
	"http://127.0.0.1:4173": true,
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
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

// LocalOnly rejects requests whose Host header is not a loopback name. The
// server listens on loopback only, so any other Host means a browser was
// pointed at it through DNS rebinding.
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch strings.ToLower(strings.Trim(host, "[]")) {
		case "localhost", "127.0.0.1", "::1":
			next.ServeHTTP(w, r)
		default:
			problem(w, http.StatusForbidden, "This demo only answers requests addressed to localhost")
		}
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
	case errors.Is(err, ErrConflict), errors.Is(err, ErrKeyConflict), errors.Is(err, ErrVersionMismatch):
		problem(w, http.StatusConflict, err.Error())
	default:
		log.Printf("unexpected error: %v", err)
		problem(w, http.StatusInternalServerError, "Internal error")
	}
}

// decode reads exactly one JSON object from a request declared as JSON.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		problem(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		// Anything after the first JSON value is rejected rather than ignored.
		if err = dec.Decode(&json.RawMessage{}); err == io.EOF {
			return true
		}
		if err == nil {
			err = errors.New("unexpected data after JSON value")
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		problem(w, http.StatusRequestEntityTooLarge, "Request body too large")
		return false
	}
	problem(w, http.StatusBadRequest, "Invalid JSON request")
	return false
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
	OrderID         string `json:"order_id"`
	Action          string `json:"action"`
	Confirmed       bool   `json:"confirmed"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
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
	if request.ExpectedVersion < 1 {
		problem(w, http.StatusBadRequest, "expected_version is required: send the order version returned by the preview")
		return
	}
	receipt, err := a.Store.ExecuteCancel(t, request.OrderID, request.ExpectedVersion, request.IdempotencyKey)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, http.StatusOK, receipt)
}

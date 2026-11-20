package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func request(t *testing.T, server http.Handler, method, path, tenant, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Demo-Tenant", tenant)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
		t.Fatalf("response is not valid JSON (%v): %s", err, w.Body)
	}
}

// cancelBody is a fully valid execute request for the given order and key.
func cancelBody(orderID, key string) string {
	return `{"order_id":"` + orderID + `","action":"cancel","confirmed":true,"expected_version":1,"idempotency_key":"` + key + `"}`
}

func TestTenantBoundary(t *testing.T) {
	h := NewHandler(NewSeedStore())
	w := request(t, h, http.MethodGet, "/api/orders/ORD-2001", "acme", "")
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body)
	}
	w = request(t, h, http.MethodGet, "/api/orders/ORD-2001", "globex", "")
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestTenantHeaderRequired(t *testing.T) {
	h := NewHandler(NewSeedStore())
	calls := []struct{ method, path, body string }{
		{http.MethodGet, "/api/orders", ""},
		{http.MethodGet, "/api/orders/ORD-1002", ""},
		{http.MethodPost, "/api/actions/preview", `{"order_id":"ORD-1002","action":"cancel"}`},
		{http.MethodPost, "/api/actions/execute", cancelBody("ORD-1002", "key-12345678")},
	}
	for _, tenant := range []string{"", "initech", "ACME"} {
		for _, c := range calls {
			if w := request(t, h, c.method, c.path, tenant, c.body); w.Code != http.StatusUnauthorized {
				t.Errorf("tenant %q %s %s: expected 401, got %d", tenant, c.method, c.path, w.Code)
			}
		}
	}
}

func TestListOrdersIsTenantScoped(t *testing.T) {
	h := NewHandler(NewSeedStore())
	w := request(t, h, http.MethodGet, "/api/orders", "globex", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body struct {
		Orders []Order `json:"orders"`
	}
	decodeBody(t, w, &body)
	if len(body.Orders) != 1 || body.Orders[0].ID != "ORD-2001" {
		t.Fatalf("globex must see only its own order, got %#v", body.Orders)
	}
}

func TestPreviewNeverMutatesAndConfirmationRequired(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	p := request(t, h, http.MethodPost, "/api/actions/preview", "acme", `{"order_id":"ORD-1002","action":"cancel"}`)
	if p.Code != 200 {
		t.Fatalf("preview: %d", p.Code)
	}
	o, _ := s.Get("acme", "ORD-1002")
	if o.Status != "processing" || o.Version != 1 {
		t.Fatal("preview unexpectedly mutated order")
	}
	w := request(t, h, http.MethodPost, "/api/actions/execute", "acme", `{"order_id":"ORD-1002","action":"cancel","idempotency_key":"test-key-123","confirmed":false}`)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCrossTenantExecuteDoesNotMutate(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	w := request(t, h, http.MethodPost, "/api/actions/execute", "acme", cancelBody("ORD-2001", "key-12345678"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body)
	}
	if o, _ := s.Get("globex", "ORD-2001"); o.Status != "processing" || o.Version != 1 {
		t.Fatalf("another tenant's order was changed: %#v", o)
	}
}

func TestExecuteRequiresPreviewedVersion(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	missing := request(t, h, http.MethodPost, "/api/actions/execute", "acme", `{"order_id":"ORD-1002","action":"cancel","confirmed":true,"idempotency_key":"key-12345678"}`)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing expected_version: expected 400, got %d", missing.Code)
	}
	stale := request(t, h, http.MethodPost, "/api/actions/execute", "acme", `{"order_id":"ORD-1002","action":"cancel","confirmed":true,"expected_version":2,"idempotency_key":"key-12345678"}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale expected_version: expected 409, got %d", stale.Code)
	}
	if o, _ := s.Get("acme", "ORD-1002"); o.Status != "processing" || o.Version != 1 {
		t.Fatalf("rejected requests must not change the order: %#v", o)
	}
	current := request(t, h, http.MethodPost, "/api/actions/execute", "acme", cancelBody("ORD-1002", "key-12345678"))
	if current.Code != http.StatusOK {
		t.Fatalf("matching expected_version: expected 200, got %d: %s", current.Code, current.Body)
	}
}

func TestIdempotentCancellation(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	body := cancelBody("ORD-1002", "key-12345678")
	w1 := request(t, h, http.MethodPost, "/api/actions/execute", "acme", body)
	w2 := request(t, h, http.MethodPost, "/api/actions/execute", "acme", body)
	if w1.Code != 200 || w2.Code != 200 {
		t.Fatalf("unexpected responses: %d %d", w1.Code, w2.Code)
	}
	var first, second Receipt
	decodeBody(t, w1, &first)
	decodeBody(t, w2, &second)
	if first.Duplicate || !second.Duplicate || second.Order.Version != 2 {
		t.Fatalf("unexpected idempotency results: %#v %#v", first, second)
	}
	mismatch := request(t, h, http.MethodPost, "/api/actions/execute", "acme", cancelBody("ORD-1001", "key-12345678"))
	if mismatch.Code != 409 {
		t.Fatalf("expected 409 on key reuse, got %d", mismatch.Code)
	}
}

func TestIdempotencyKeysAreTenantScoped(t *testing.T) {
	h := NewHandler(NewSeedStore())
	for tenant, orderID := range map[string]string{"acme": "ORD-1002", "globex": "ORD-2001"} {
		w := request(t, h, http.MethodPost, "/api/actions/execute", tenant, cancelBody(orderID, "shared-key-0001"))
		var receipt Receipt
		decodeBody(t, w, &receipt)
		if w.Code != http.StatusOK || receipt.Duplicate || receipt.Order.ID != orderID {
			t.Fatalf("%s: a key used by another tenant must not collide: %d %#v", tenant, w.Code, receipt)
		}
	}
}

func TestParallelCancellationOnlyOneWins(t *testing.T) {
	s := NewSeedStore()
	var wg sync.WaitGroup
	success := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "unique-key-" + strings.Repeat("a", i+1)
			_, err := s.ExecuteCancel("acme", "ORD-1002", 1, key)
			success <- err == nil
		}(i)
	}
	wg.Wait()
	close(success)
	n := 0
	for ok := range success {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected one cancellation, got %d", n)
	}
	if o, _ := s.Get("acme", "ORD-1002"); o.Status != "cancelled" || o.Version != 2 {
		t.Fatalf("expected exactly one mutation, got %#v", o)
	}
}

func TestAlreadyDeliveredCannotCancel(t *testing.T) {
	s := NewSeedStore()
	p, err := s.PreviewCancel("acme", "ORD-1001")
	if err != nil || p.CanExecute {
		t.Fatal("delivered order must not be cancellable")
	}
	if _, err := s.ExecuteCancel("acme", "ORD-1001", 1, "key-12345678"); err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestRequestValidation(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	const preview, execute = "/api/actions/preview", "/api/actions/execute"
	cases := []struct {
		name, path, body string
		want             int
	}{
		{"unknown action", preview, `{"order_id":"ORD-1002","action":"refund"}`, 400},
		{"missing order id", preview, `{"action":"cancel"}`, 400},
		{"unknown field", preview, `{"order_id":"ORD-1002","action":"cancel","admin":true}`, 400},
		{"malformed JSON", preview, `{"order_id":`, 400},
		{"empty body", preview, ``, 400},
		{"trailing data", preview, `{"order_id":"ORD-1002","action":"cancel"} extra`, 400},
		{"second JSON value", preview, `{"order_id":"ORD-1002","action":"cancel"}{}`, 400},
		{"oversized body", preview, `{"order_id":"` + strings.Repeat("a", maxBodyBytes) + `","action":"cancel"}`, 413},
		{"short idempotency key", execute, cancelBody("ORD-1002", "short"), 400},
		{"padded idempotency key", execute, cancelBody("ORD-1002", " key-12345678"), 400},
	}
	for _, c := range cases {
		w := request(t, h, http.MethodPost, c.path, "acme", c.body)
		var body map[string]string
		decodeBody(t, w, &body)
		if w.Code != c.want || body["error"] == "" {
			t.Errorf("%s: expected %d with a JSON error, got %d: %s", c.name, c.want, w.Code, w.Body)
		}
	}

	r := httptest.NewRequest(http.MethodPost, preview, strings.NewReader(`{"order_id":"ORD-1002","action":"cancel"}`))
	r.Header.Set("X-Demo-Tenant", "acme")
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("non-JSON content type: expected 415, got %d", w.Code)
	}

	if o, _ := s.Get("acme", "ORD-1002"); o.Status != "processing" || o.Version != 1 {
		t.Fatalf("invalid requests must not change the order: %#v", o)
	}
}

func TestUnknownRoutesAndMethodsReturnJSON(t *testing.T) {
	h := NewHandler(NewSeedStore())
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/nope", 404},
		{http.MethodGet, "/api/orders/", 404},
		{http.MethodGet, "/api/actions/execute", 405},
		{http.MethodPost, "/api/orders/ORD-1002", 405},
		{http.MethodDelete, "/api/orders", 405},
	}
	for _, c := range cases {
		w := request(t, h, c.method, c.path, "acme", "")
		var body map[string]string
		decodeBody(t, w, &body)
		if w.Code != c.want || body["error"] == "" {
			t.Errorf("%s %s: expected %d with a JSON error, got %d: %s", c.method, c.path, c.want, w.Code, w.Body)
		}
	}
	if allow := request(t, h, http.MethodGet, "/api/actions/execute", "acme", "").Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("405 must name the allowed method, got Allow=%q", allow)
	}
}

func TestCORS(t *testing.T) {
	h := NewHandler(NewSeedStore())
	allowOrigin := func(method, origin string) (int, http.Header) {
		r := httptest.NewRequest(method, "/api/health", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Header()
	}
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:4173", "http://127.0.0.1:4173"} {
		if _, header := allowOrigin(http.MethodGet, origin); header.Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("%s should be an allowed origin", origin)
		}
	}
	for _, origin := range []string{"http://evil.example", "http://localhost:3000", "https://localhost:5173"} {
		if _, header := allowOrigin(http.MethodGet, origin); header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s must not be an allowed origin", origin)
		}
	}
	code, header := allowOrigin(http.MethodOptions, "http://localhost:5173")
	if code != http.StatusNoContent || !strings.Contains(header.Get("Access-Control-Allow-Headers"), "X-Demo-Tenant") {
		t.Errorf("preflight: expected 204 allowing X-Demo-Tenant, got %d %v", code, header)
	}
}

func TestLocalOnly(t *testing.T) {
	h := LocalOnly(NewHandler(NewSeedStore()))
	hosts := map[string]int{
		"localhost:8090":                  200,
		"LOCALHOST:8090":                  200,
		"localhost":                       200,
		"127.0.0.1:8090":                  200,
		"[::1]:8090":                      200,
		"attacker.example":                403,
		"attacker.example:8090":           403,
		"192.168.1.20:8090":               403,
		"localhost.attacker.example:8090": 403,
	}
	for host, want := range hosts {
		r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("Host %q: expected %d, got %d", host, want, w.Code)
		}
	}
}

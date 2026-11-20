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

func TestPreviewNeverMutatesAndConfirmationRequired(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	p := request(t, h, http.MethodPost, "/api/actions/preview", "acme", `{"order_id":"ORD-1002","action":"cancel"}`)
	if p.Code != 200 {
		t.Fatalf("preview: %d", p.Code)
	}
	o, _ := s.Get("acme", "ORD-1002")
	if o.Status != "processing" {
		t.Fatal("preview unexpectedly mutated order")
	}
	w := request(t, h, http.MethodPost, "/api/actions/execute", "acme", `{"order_id":"ORD-1002","action":"cancel","idempotency_key":"test-key-123","confirmed":false}`)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestIdempotentCancellation(t *testing.T) {
	s := NewSeedStore()
	h := NewHandler(s)
	body := `{"order_id":"ORD-1002","action":"cancel","idempotency_key":"key-12345678","confirmed":true}`
	w1 := request(t, h, http.MethodPost, "/api/actions/execute", "acme", body)
	w2 := request(t, h, http.MethodPost, "/api/actions/execute", "acme", body)
	if w1.Code != 200 || w2.Code != 200 {
		t.Fatalf("unexpected responses: %d %d", w1.Code, w2.Code)
	}
	var first, second Receipt
	_ = json.Unmarshal(w1.Body.Bytes(), &first)
	_ = json.Unmarshal(w2.Body.Bytes(), &second)
	if first.Duplicate || !second.Duplicate || second.Order.Version != 2 {
		t.Fatalf("unexpected idempotency results: %#v %#v", first, second)
	}
	mismatch := request(t, h, http.MethodPost, "/api/actions/execute", "acme", `{"order_id":"ORD-1001","action":"cancel","idempotency_key":"key-12345678","confirmed":true}`)
	if mismatch.Code != 409 {
		t.Fatalf("expected 409 on key reuse, got %d", mismatch.Code)
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
			_, err := s.ExecuteCancel("acme", "ORD-1002", key)
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
}

func TestAlreadyDeliveredCannotCancel(t *testing.T) {
	s := NewSeedStore()
	p, err := s.PreviewCancel("acme", "ORD-1001")
	if err != nil || p.CanExecute {
		t.Fatal("delivered order must not be cancellable")
	}
}

package agent

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotFound    = errors.New("order not found")
	ErrConflict    = errors.New("order cannot be cancelled")
	ErrKeyConflict = errors.New("idempotency key already used for a different operation")
)

type Order struct {
	ID       string  `json:"id"`
	Tenant   string  `json:"tenant"`
	Product  string  `json:"product"`
	Status   string  `json:"status"`
	TotalUSD float64 `json:"total_usd"`
	Version  int     `json:"version"`
}

type Preview struct {
	Order                Order  `json:"order"`
	CanExecute           bool   `json:"can_execute"`
	ConfirmationRequired bool   `json:"confirmation_required"`
	Reason               string `json:"reason"`
}

type Receipt struct {
	Order     Order  `json:"order"`
	Duplicate bool   `json:"duplicate"`
	Message   string `json:"message"`
}

type operation struct {
	tenant  string
	orderID string
	action  string
	receipt Receipt
}

type Store struct {
	mu         sync.Mutex
	orders     map[string]Order
	operations map[string]operation
}

func NewSeedStore() *Store {
	return &Store{
		orders: map[string]Order{
			"ORD-1001": {ID: "ORD-1001", Tenant: "acme", Product: "Wireless Headphones", Status: "delivered", TotalUSD: 129.00, Version: 1},
			"ORD-1002": {ID: "ORD-1002", Tenant: "acme", Product: "Bluetooth Speaker", Status: "processing", TotalUSD: 79.00, Version: 1},
			"ORD-2001": {ID: "ORD-2001", Tenant: "globex", Product: "Smart Keyboard", Status: "processing", TotalUSD: 149.00, Version: 1},
		},
		operations: map[string]operation{},
	}
}

func (s *Store) List(tenant string) []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	orders := make([]Order, 0)
	for _, o := range s.orders {
		if o.Tenant == tenant {
			orders = append(orders, o)
		}
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].ID < orders[j].ID })
	return orders
}

func (s *Store) Get(tenant, id string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(tenant, id)
}

func (s *Store) getLocked(tenant, id string) (Order, error) {
	o, ok := s.orders[id]
	if !ok || o.Tenant != tenant {
		return Order{}, ErrNotFound
	}
	return o, nil
}

func (s *Store) PreviewCancel(tenant, id string) (Preview, error) {
	o, err := s.Get(tenant, id)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{Order: o, CanExecute: o.Status == "processing", ConfirmationRequired: true}
	if p.CanExecute {
		p.Reason = "Cancellation is permitted after explicit confirmation."
	} else {
		p.Reason = "Only processing orders can be cancelled."
	}
	return p, nil
}

// ExecuteCancel is atomic within one Go process. A production multi-instance
// service needs a durable transactional store for both the mutation and receipt.
func (s *Store) ExecuteCancel(tenant, id, idempotencyKey string) (Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tenant + ":" + idempotencyKey
	if existing, ok := s.operations[key]; ok {
		if existing.tenant != tenant || existing.orderID != id || existing.action != "cancel" {
			return Receipt{}, ErrKeyConflict
		}
		r := existing.receipt
		r.Duplicate = true
		return r, nil
	}
	o, err := s.getLocked(tenant, id)
	if err != nil {
		return Receipt{}, err
	}
	if o.Status != "processing" {
		return Receipt{}, ErrConflict
	}
	o.Status = "cancelled"
	o.Version++
	s.orders[id] = o
	receipt := Receipt{Order: o, Duplicate: false, Message: "Order cancelled successfully."}
	s.operations[key] = operation{tenant: tenant, orderID: id, action: "cancel", receipt: receipt}
	return receipt, nil
}

func ValidTenant(s string) bool {
	return s == "acme" || s == "globex"
}

func ValidKey(s string) bool {
	return len(s) >= 8 && len(s) <= 128 && strings.TrimSpace(s) == s
}

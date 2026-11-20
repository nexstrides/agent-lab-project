# Sierra-inspired Engineering Lab — Project 1: Enterprise Action Agent

**Original educational implementation. Not affiliated with Sierra.**

## Milestone 1: guarded enterprise actions

A local full-stack example demonstrating:
- Tenant-scoped data access using a **demo-only header** (not authentication)
- Read-only order lookup and preview
- Explicit human confirmation for a cancellation, bound to the order version that was previewed
- Server-enforced business eligibility rules
- Idempotency receipts that prevent duplicate mutations after retries
- Mutex-protected in-process concurrency and Go regression tests
- React + TypeScript interactive interface

**Important:** There is NO LLM, MCP server, RAG system, production auth, or database yet. Those are milestones 2–5. The tenant selector is not a secure identity system. An attacker can spoof the header. Do not expose this prototype to the public internet or use real customer records. For that reason the server listens on the loopback interface only (`127.0.0.1` and `::1`) and rejects requests whose `Host` header is not a loopback name.

## Requirements
- Go 1.23+
- Node.js 22+ and npm

## Run locally — two terminals

Terminal A (this also serves a zero-dependency browser demo at `http://localhost:8090`):

```bash
cd backend
go test -race ./...
go run ./cmd/server
```

Terminal B:

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173` for the React/TypeScript UI. If you only want the Go-powered, dependency-free demo, open `http://localhost:8090` and skip Terminal B. Both UIs call the same backend.

`go test -race` needs cgo and a C compiler. On Windows without gcc it stops with `-race requires cgo`; run `go test ./...` there instead. CI runs the race detector on Linux for every push.

To check the production build of the UI, run `npm run build` and then `npm run preview`, and open `http://localhost:4173`.

## API routes

Every route except health needs the header `X-Demo-Tenant: acme` or `globex`. Errors are always JSON: `{ "error": "..." }`.

- `GET /api/health`
- `GET /api/orders` lists the tenant's orders
- `GET /api/orders/{id}`
- `POST /api/actions/preview` with `{ "order_id": "ORD-1002", "action": "cancel" }`
- `POST /api/actions/execute` with `{ "order_id": "ORD-1002", "action": "cancel", "confirmed": true, "expected_version": 1, "idempotency_key": "unique-value-at-least-8-chars" }`

`expected_version` is the `version` the preview returned. If the order has changed since the preview, execute answers `409` and the order must be previewed again. Request bodies must be sent as `Content-Type: application/json`.

Run `bash scripts/smoke.sh` in the project root after starting the API.

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` or `develop` and on every pull request:

- **Backend:** `gofmt` check, `go vet`, `go test -race`, then starts the server and runs `scripts/smoke.sh` against it.
- **Frontend:** `npm ci`, then `npm run build` (typecheck and production build).

## Architecture boundaries

`React/Vite → Go REST API → authorization/eligibility check → in-memory orders/receipts`.

In the next milestones, we will add authenticated identities, a durable transactional store, a model that can propose actions but not authorize them, and an MCP integration. Model calls must never bypass the deterministic authorization layer.

## Relevant Sierra engineering articles
- https://sierra.ai/blog/agent-development-life-cycle
- https://sierra.ai/blog/building-sierras-mcp-gateway-an-engineering-iceberg
- https://sierra.ai/blog/agency-secure-scalable-sandboxes-for-agents

Read `ROADMAP.md` for the 3-project plan.

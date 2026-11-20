# Sierra-inspired Engineering Lab — Project 1: Enterprise Action Agent

**Original educational implementation. Not affiliated with Sierra.**

## Milestone 1: guarded enterprise actions

A local full-stack example demonstrating:
- Tenant-scoped data access using a **demo-only header** (not authentication)
- Read-only order lookup and preview
- Explicit human confirmation for a cancellation
- Server-enforced business eligibility rules
- Idempotency receipts that prevent duplicate mutations after retries
- Mutex-protected in-process concurrency and Go regression tests
- React + TypeScript interactive interface

**Important:** There is NO LLM, MCP server, RAG system, production auth, or database yet. Those are milestones 2–5. The tenant selector is not a secure identity system. An attacker can spoof the header. Do not expose this prototype to the public internet or use real customer records.

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

## API routes

- `GET /api/health`
- `GET /api/orders/{id}` (header `X-Demo-Tenant: acme` or `globex`)
- `POST /api/actions/preview` with `{ "order_id": "ORD-1002", "action": "cancel" }`
- `POST /api/actions/execute` with `{ "order_id": "ORD-1002", "action": "cancel", "confirmed": true, "idempotency_key": "unique-value-at-least-8-chars" }`

Run `bash scripts/smoke.sh` in the project root after starting the API.

## Architecture boundaries

`React/Vite → Go REST API → authorization/eligibility check → in-memory orders/receipts`.

In the next milestones, we will add authenticated identities, a durable transactional store, a model that can propose actions but not authorize them, and an MCP integration. Model calls must never bypass the deterministic authorization layer.

## Relevant Sierra engineering articles
- https://sierra.ai/blog/agent-development-life-cycle
- https://sierra.ai/blog/building-sierras-mcp-gateway-an-engineering-iceberg
- https://sierra.ai/blog/agency-secure-scalable-sandboxes-for-agents

Read `ROADMAP.md` for the 3-project plan.

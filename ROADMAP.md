# Learning Roadmap (three original Sierra-inspired projects)

## Project 1 — Enterprise Action Agent [IN PROGRESS]

- [x] M1: Local Go API + React UI; tenant-scoped demo records; preview → confirm → execute; idempotent cancel; tests
- [ ] M2: Real user auth, scoped permissions, Postgres transactions, durable idempotency records
- [ ] M3: MCP tool server/client, typed tool schema, sanitized output, explicit tool authorization
- [ ] M4: LLM reasoning + RAG with pgvector and source citations; fail-closed action safety
- [ ] M5: OpenTelemetry traces, offline evals, prompt-injection tests, Docker Compose, CI

M1 acceptance: `go test -race ./...` passes; preview cannot mutate; cross-tenant lookup returns 404; unconfirmed cancel returns 403; replay of same idempotency key returns prior receipt and does not increment version; racing unique cancel requests causes only one mutation.

## Project 2 — Agent Evaluation & Release Studio [NEXT]

- [ ] Evaluation case schema and versioned datasets
- [ ] Conversation simulator with mocked tools
- [ ] Deterministic and model-graded scorers
- [ ] Baseline-vs-candidate comparison and trace inspection
- [ ] Approval gates, immutable releases, and canary deployment simulator

Suggested stack: Python/FastAPI, pytest, React/TypeScript, PostgreSQL, OpenTelemetry.

## Project 3 — Durable Agent Sandbox Platform [LATER]

- [ ] Runner control plane and state machine
- [ ] Redis Streams input/output, checkpoint records and deterministic replay
- [ ] Docker worker isolation and resource limits
- [ ] Kubernetes pod provisioning and lifecycle controls
- [ ] Hibernation, crash recovery, structured tracing, stress tests

Suggested stack: Go, Kubernetes, Redis Streams, persistent storage, OpenTelemetry.

Both projects are inspired by public Sierra architecture concepts, not its source code.

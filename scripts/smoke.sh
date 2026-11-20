#!/usr/bin/env bash
set -euo pipefail
BASE="${BASE:-http://localhost:8090}"
echo "Health check:"
curl -fsS "$BASE/api/health"; echo
echo "Order lookup:"
curl -fsS -H 'X-Demo-Tenant: acme' "$BASE/api/orders/ORD-1002"; echo
echo "Preview (read only):"
curl -fsS -X POST -H 'Content-Type: application/json' -H 'X-Demo-Tenant: acme' -d '{"order_id":"ORD-1002","action":"cancel"}' "$BASE/api/actions/preview"; echo
echo "Execute (requires confirmation):"
curl -fsS -X POST -H 'Content-Type: application/json' -H 'X-Demo-Tenant: acme' -d '{"order_id":"ORD-1002","action":"cancel","idempotency_key":"smoke-unique-00001","confirmed":true}' "$BASE/api/actions/execute"; echo
echo "Replay same key (duplicate:true):"
curl -fsS -X POST -H 'Content-Type: application/json' -H 'X-Demo-Tenant: acme' -d '{"order_id":"ORD-1002","action":"cancel","idempotency_key":"smoke-unique-00001","confirmed":true}' "$BASE/api/actions/execute"; echo

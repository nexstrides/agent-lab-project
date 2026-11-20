#!/usr/bin/env bash
set -euo pipefail
BASE="${BASE:-http://localhost:8090}"
EXECUTE='{"order_id":"ORD-1002","action":"cancel","expected_version":1,"idempotency_key":"smoke-unique-00001","confirmed":true}'
echo "Health check:"
curl -fsS "$BASE/api/health"; echo
echo "Order lookup:"
curl -fsS -H 'X-Demo-Tenant: acme' "$BASE/api/orders/ORD-1002"; echo
echo "Preview (read only):"
curl -fsS -X POST -H 'Content-Type: application/json' -H 'X-Demo-Tenant: acme' -d '{"order_id":"ORD-1002","action":"cancel"}' "$BASE/api/actions/preview"; echo
echo "Execute (requires confirmation and the previewed version):"
curl -fsS -X POST -H 'Content-Type: application/json' -H 'X-Demo-Tenant: acme' -d "$EXECUTE" "$BASE/api/actions/execute"; echo
echo "Replay same key (duplicate:true):"
curl -fsS -X POST -H 'Content-Type: application/json' -H 'X-Demo-Tenant: acme' -d "$EXECUTE" "$BASE/api/actions/execute"; echo

#!/bin/sh
# ============================================================================
# HiveMind E2E Integration Tests
# ============================================================================
# Runs against a live HiveMind API server.
# Exit code 0 = all tests passed, 1 = failures detected.
# ============================================================================

set +e

API="${API_URL:-http://localhost:8080}"
PASS=0
FAIL=0
TOTAL=0

# ── Helpers ──────────────────────────────────────────────────────────────────

green() { printf "\033[32m%s\033[0m" "$1"; }
red()   { printf "\033[31m%s\033[0m" "$1"; }
bold()  { printf "\033[1m%s\033[0m" "$1"; }

assert_status() {
    test_name="$1"
    expected="$2"
    actual="$3"
    TOTAL=$((TOTAL + 1))

    if [ "$actual" = "$expected" ]; then
        PASS=$((PASS + 1))
        printf "  $(green "PASS") %s (HTTP %s)\n" "$test_name" "$actual"
    else
        FAIL=$((FAIL + 1))
        printf "  $(red "FAIL") %s (expected %s, got %s)\n" "$test_name" "$expected" "$actual"
    fi
}

assert_contains() {
    test_name="$1"
    body="$2"
    expected="$3"
    TOTAL=$((TOTAL + 1))

    if echo "$body" | grep -q "$expected"; then
        PASS=$((PASS + 1))
        printf "  $(green "PASS") %s (contains '%s')\n" "$test_name" "$expected"
    else
        FAIL=$((FAIL + 1))
        printf "  $(red "FAIL") %s (missing '%s')\n" "$test_name" "$expected"
        printf "       Body: %.200s\n" "$body"
    fi
}

assert_not_empty() {
    test_name="$1"
    body="$2"
    TOTAL=$((TOTAL + 1))

    if [ -n "$body" ] && [ "$body" != "{}" ] && [ "$body" != "null" ]; then
        PASS=$((PASS + 1))
        printf "  $(green "PASS") %s (non-empty response)\n" "$test_name"
    else
        FAIL=$((FAIL + 1))
        printf "  $(red "FAIL") %s (empty response)\n" "$test_name"
    fi
}

# ── Wait for server ─────────────────────────────────────────────────────────

printf "\n$(bold '🐝 HiveMind E2E Integration Tests')\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "Target: %s\n\n" "$API"

printf "⏳ Waiting for server...\n"
retries=0
max_retries=30
while [ $retries -lt $max_retries ]; do
    if curl -sf "${API}/health" > /dev/null 2>&1; then
        printf "✅ Server is up!\n\n"
        break
    fi
    retries=$((retries + 1))
    sleep 1
done

if [ $retries -eq $max_retries ]; then
    printf "$(red '❌ Server failed to start after %d seconds')\n" "$max_retries"
    exit 1
fi

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 1: Health & Metrics
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Health & Metrics')\n"

# T1: GET /health returns 200
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/health")
assert_status "GET /health returns 200" "200" "$status"

# T2: Health response contains status:ok
body=$(curl -sf "${API}/health")
assert_contains "Health response has status ok" "$body" '"status":"ok"'

# T3: Health response has worker_healthy field
assert_contains "Health response has worker_healthy" "$body" '"worker_healthy"'

# T4: GET /metrics returns 200
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/metrics")
assert_status "GET /metrics returns 200" "200" "$status"

# T5: Metrics response is non-empty
body=$(curl -sf "${API}/metrics")
assert_not_empty "Metrics response is non-empty" "$body"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 2: Models API
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Models API')\n"

# T6: GET /v1/models returns 200
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/v1/models")
assert_status "GET /v1/models returns 200" "200" "$status"

# T7: Models response contains data array
body=$(curl -sf "${API}/v1/models")
assert_contains "Models response has data array" "$body" '"data"'

# T8: Models response has model objects
assert_contains "Models response has object field" "$body" '"object"'

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 3: Room Management
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Room Management')\n"

# T9: POST /room/create returns 201 Created
body=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{"model_id":"mock-7b"}')
status=$(echo "$body" | tail -1)
body_content=$(echo "$body" | sed '$d')
assert_status "POST /room/create returns 201" "201" "$status"

# T10: Create room response has invite_code
assert_contains "Create room has invite_code" "$body_content" '"invite_code"'

# T11: GET /room/status returns data
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/room/status")
assert_status "GET /room/status returns 200" "200" "$status"

# T12: Room status has room data
body=$(curl -sf "${API}/room/status")
assert_contains "Room status has room field" "$body" '"room"'

# T13: Multi-room: second room creation also returns 201 (multi-room support)
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{"model_id":"mock-7b"}')
assert_status "Second room create returns 201 (multi-room)" "201" "$status"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 4: Chat Completions (Non-Streaming)
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Chat Completions')\n"

# T13: POST /v1/chat/completions returns 200
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock","messages":[{"role":"user","content":"hello"}]}')
assert_status "POST /v1/chat/completions returns 200" "200" "$status"

# T14: Chat response has choices
body=$(curl -sf \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock","messages":[{"role":"user","content":"test"}]}')
assert_contains "Chat response has choices" "$body" '"choices"'

# T15: Chat response has usage
assert_contains "Chat response has usage" "$body" '"usage"'

# T16: Empty messages returns 400
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock","messages":[]}')
assert_status "Empty messages returns 400" "400" "$status"

# T17: Invalid JSON returns 400
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d 'not-json')
assert_status "Invalid JSON returns 400" "400" "$status"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 5: Streaming Chat
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Streaming Chat (SSE)')\n"

# T18: Streaming returns text/event-stream
content_type=$(curl -s -o /dev/null -w "%{content_type}" --max-time 15 \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock","messages":[{"role":"user","content":"hi"}],"stream":true}')
TOTAL=$((TOTAL + 1))
if echo "$content_type" | grep -qi "text/event-stream"; then
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') Streaming returns text/event-stream\n"
else
    FAIL=$((FAIL + 1))
    printf "  $(red 'FAIL') Streaming content-type: %s\n" "$content_type"
fi

# T19: Streaming response contains data: prefix
stream_body=$(curl -sf --max-time 10 \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock","messages":[{"role":"user","content":"hi"}],"stream":true}')
assert_contains "Streaming has data: prefix" "$stream_body" "data:"

# T20: Streaming ends with [DONE]
assert_contains "Streaming ends with [DONE]" "$stream_body" "[DONE]"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 6: Image Generation
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Image Generation')\n"

# T21: POST /v1/images/generations returns 200
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock-diffusion","prompt":"a golden bee"}')
assert_status "POST /v1/images/generations returns 200" "200" "$status"

# T22: Image response has data array
body=$(curl -sf \
    -X POST "${API}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock-diffusion","prompt":"a honeycomb"}')
assert_contains "Image response has data" "$body" '"data"'

# T23: Empty prompt returns 400
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{"model":"mock-diffusion","prompt":""}')
assert_status "Empty prompt returns 400" "400" "$status"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 7: Web Dashboard (SPA)
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Web Dashboard')\n"

# T24: Root serves HTML (SPA)
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/")
assert_status "GET / returns 200" "200" "$status"

root_body=$(curl -sf "${API}/")
assert_contains "Root serves HTML" "$root_body" "<!doctype html>"

# T25: API status endpoint works
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/api/health")
assert_status "GET /api/health returns 200" "200" "$status"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 8: Expanded Catalog (20 models, 5 types)
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Expanded Catalog')\n"

# T-CAT1: GET /v1/models/catalog returns all 20 models
catalog_body=$(curl -sf "${API}/v1/models/catalog")
assert_contains "Catalog has models array" "$catalog_body" '"models"'

# Count models by parameter_size (unique to model entries)
model_count=$(echo "$catalog_body" | grep -o '"parameter_size"' | wc -l)
TOTAL=$((TOTAL + 1))
if [ "$model_count" -eq 20 ]; then
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') Catalog has 20 models (got %d)\n" "$model_count"
else
    FAIL=$((FAIL + 1))
    printf "  $(red 'FAIL') Expected 20 models, got %d\n" "$model_count"
fi

# T-CAT2: New model types present
assert_contains "Catalog has code models" "$catalog_body" '"type":"code"'
assert_contains "Catalog has embedding models" "$catalog_body" '"type":"embedding"'
assert_contains "Catalog has multimodal models" "$catalog_body" '"type":"multimodal"'
assert_contains "Catalog has llm models" "$catalog_body" '"type":"llm"'
assert_contains "Catalog has diffusion models" "$catalog_body" '"type":"diffusion"'

# T-CAT3: Specific new models present
assert_contains "DeepSeek Coder V2 236B in catalog" "$catalog_body" '"deepseek-ai/DeepSeek-Coder-V2-236B"'
assert_contains "Qwen2-VL 72B in catalog" "$catalog_body" '"Qwen/Qwen2-VL-72B"'
assert_contains "Nomic Embed in catalog" "$catalog_body" '"nomic-ai/nomic-embed-text-v1.5"'
assert_contains "StarCoder2 15B in catalog" "$catalog_body" '"bigcode/StarCoder2-15B"'

# T-CAT4: Suggestion for very high VRAM → largest model
suggest_body=$(curl -sf "${API}/v1/models/catalog?vram_mb=200000")
assert_contains "Suggestion for 200GB VRAM is DeepSeek 236B" "$suggest_body" '"deepseek-ai/DeepSeek-Coder-V2-236B"'

# T-CAT5: Suggestion for small VRAM → embedding model
suggest_body=$(curl -sf "${API}/v1/models/catalog?vram_mb=600")
assert_contains "Suggestion for 600MB is Nomic Embed" "$suggest_body" '"nomic-ai/nomic-embed-text-v1.5"'

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 9: Resource Donation via API
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Resource Donation')\n"

# Leave current room first (from earlier tests)
curl -sf -X DELETE "${API}/room/leave" > /dev/null 2>&1

# T-DON1: Create room with donation_pct preserved in response
body=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "TinyLlama/TinyLlama-1.1B",
        "model_type": "llm",
        "max_peers": 5,
        "resources": {
            "gpu_name": "NVIDIA RTX 3060",
            "vram_total_mb": 12288,
            "vram_free_mb": 10240,
            "ram_total_mb": 32768,
            "ram_free_mb": 24576,
            "cuda_available": true,
            "platform": "Linux",
            "donation_pct": 50
        }
    }')
status=$(echo "$body" | tail -1)
body_content=$(echo "$body" | sed '$d')
assert_status "Create with donation_pct returns 201" "201" "$status"
assert_contains "Donation pct preserved" "$body_content" '"donation_pct":50'
assert_contains "Room is active (50% of 9728 > 2048)" "$body_content" '"state":"active"'

# Leave to test next scenario
curl -sf -X DELETE "${API}/room/leave" > /dev/null 2>&1

# T-DON2: Create with code model auto-fills type
body=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{"model_id": "Qwen/Qwen2.5-Coder-7B", "max_peers": 3}')
status=$(echo "$body" | tail -1)
body_content=$(echo "$body" | sed '$d')
assert_status "Create with code model returns 201" "201" "$status"
assert_contains "Auto-filled type is code" "$body_content" '"model_type":"code"'

# Leave to test join
curl -sf -X DELETE "${API}/room/leave" > /dev/null 2>&1

# T-DON3: Join with donation_pct in resources
body=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/join" \
    -H "Content-Type: application/json" \
    -d '{
        "invite_code": "test-donation-join",
        "resources": {
            "gpu_name": "NVIDIA RTX 3070",
            "vram_total_mb": 8192,
            "vram_free_mb": 6144,
            "ram_total_mb": 16384,
            "ram_free_mb": 12288,
            "cuda_available": true,
            "platform": "Windows",
            "donation_pct": 75
        }
    }')
status=$(echo "$body" | tail -1)
body_content=$(echo "$body" | sed '$d')
assert_status "Join with donation returns 200" "200" "$status"
assert_contains "Join preserves donation_pct" "$body_content" '"donation_pct":75'
assert_contains "Joined room has valid state" "$body_content" '"state":'
assert_contains "Self has layers" "$body_content" '"layers"'

# Leave for cleanup
curl -sf -X DELETE "${API}/room/leave" > /dev/null 2>&1

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 10: Error Handling & Edge Cases
# ═══════════════════════════════════════════════════════════════════════════

# Ensure a room exists for chat-related tests below
curl -sf -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{"model_id":"TinyLlama/TinyLlama-1.1B","max_peers":3}' > /dev/null 2>&1

printf "$(bold '▸ Error Handling')\n"

# T26: Unknown route returns SPA (200) — SPA catch-all
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/nonexistent-route")
assert_status "Unknown route returns SPA fallback" "200" "$status"

# T27: CORS headers present (check Allow-Methods, always set regardless of Origin)
cors=$(curl -s -o /dev/null -D - -X OPTIONS "${API}/v1/models" | grep -i "access-control-allow-methods" | head -1)
TOTAL=$((TOTAL + 1))
if echo "$cors" | grep -qi "access-control"; then
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') CORS headers present\n"
else
    FAIL=$((FAIL + 1))
    printf "  $(red 'FAIL') CORS headers missing\n"
fi

# T28: Large payload within limits
large_msg=$(printf '{"model":"mock","messages":[{"role":"user","content":"%.0s"}]}' $(seq 1 100))
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "$large_msg")
assert_status "Large payload accepted" "200" "$status"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST SUITE 9: Concurrent Requests
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '▸ Concurrent Requests')\n"

# T29: 10 parallel health checks
TOTAL=$((TOTAL + 1))
pids=""
fail_concurrent=0
for i in $(seq 1 10); do
    curl -sf "${API}/health" > /dev/null 2>&1 &
    pids="$pids $!"
done

for pid in $pids; do
    if ! wait "$pid"; then
        fail_concurrent=$((fail_concurrent + 1))
    fi
done

if [ "$fail_concurrent" -eq 0 ]; then
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') 10 concurrent health checks OK\n"
else
    FAIL=$((FAIL + 1))
    printf "  $(red 'FAIL') %d/10 concurrent requests failed\n" "$fail_concurrent"
fi

# T30: 5 parallel chat completions
TOTAL=$((TOTAL + 1))
pids=""
fail_concurrent=0
for i in $(seq 1 5); do
    curl -sf -X POST "${API}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{\"model\":\"mock\",\"messages\":[{\"role\":\"user\",\"content\":\"parallel test $i\"}]}" \
        > /dev/null 2>&1 &
    pids="$pids $!"
done

for pid in $pids; do
    if ! wait "$pid"; then
        fail_concurrent=$((fail_concurrent + 1))
    fi
done

if [ "$fail_concurrent" -eq 0 ]; then
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') 5 concurrent chat completions OK\n"
else
    FAIL=$((FAIL + 1))
    printf "  $(red 'FAIL') %d/5 concurrent chat requests failed\n" "$fail_concurrent"
fi

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# RESULTS
# ═══════════════════════════════════════════════════════════════════════════

printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "$(bold 'Results:') %s passed, %s failed, %s total\n" \
    "$(green "$PASS")" \
    "$(if [ "$FAIL" -gt 0 ]; then red "$FAIL"; else echo "$FAIL"; fi)" \
    "$TOTAL"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"

if [ "$FAIL" -gt 0 ]; then
    printf "\n$(red '❌ E2E TESTS FAILED')\n\n"
    exit 1
else
    printf "\n$(green '✅ ALL E2E TESTS PASSED')\n\n"
    exit 0
fi

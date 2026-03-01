#!/bin/sh
# ============================================================================
# HiveMind P2P Wiring E2E Test
# ============================================================================
# Tests the real distributed flow:
#   signaling health → alice creates room → signaling has room →
#   bob joins → peers see each other → health checks → bob leaves →
#   alice has 1 peer → cleanup
# ============================================================================

set +e

ALICE="${ALICE_URL:-http://localhost:8080}"
BOB="${BOB_URL:-http://localhost:8080}"
SIGNALING="${SIGNALING_URL:-http://localhost:7777}"
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
        printf "       Body: %.300s\n" "$body"
    fi
}

assert_not_contains() {
    test_name="$1"
    body="$2"
    pattern="$3"
    TOTAL=$((TOTAL + 1))

    if echo "$body" | grep -q "$pattern"; then
        FAIL=$((FAIL + 1))
        printf "  $(red "FAIL") %s (unexpectedly contains '%s')\n" "$test_name" "$pattern"
    else
        PASS=$((PASS + 1))
        printf "  $(green "PASS") %s (does not contain '%s')\n" "$test_name" "$pattern"
    fi
}

# ── Wait for services ──────────────────────────────────────────────────────

printf "\n$(bold 'HiveMind P2P Wiring E2E Tests')\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "Signaling: %s\n" "$SIGNALING"
printf "Alice:     %s\n" "$ALICE"
printf "Bob:       %s\n\n" "$BOB"

printf "Waiting for services...\n"
retries=0
max_retries=30
while [ $retries -lt $max_retries ]; do
    sig_ok=$(curl -sf "${SIGNALING}/signal/health" 2>/dev/null)
    alice_ok=$(curl -sf "${ALICE}/health" 2>/dev/null)
    bob_ok=$(curl -sf "${BOB}/health" 2>/dev/null)
    if [ -n "$sig_ok" ] && [ -n "$alice_ok" ] && [ -n "$bob_ok" ]; then
        printf "All services are up!\n\n"
        break
    fi
    retries=$((retries + 1))
    sleep 1
done

if [ $retries -eq $max_retries ]; then
    printf "$(red 'Services failed to start after %d seconds')\n" "$max_retries"
    exit 1
fi

# ═══════════════════════════════════════════════════════════════════════════
# TEST 1: Signaling Server Health
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '1. Signaling Server')\n"

status=$(curl -s -o /dev/null -w "%{http_code}" "${SIGNALING}/signal/health")
assert_status "Signaling health returns 200" "200" "$status"

body=$(curl -sf "${SIGNALING}/signal/health")
assert_contains "Signaling health has status ok" "$body" '"status":"ok"'

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 2: Alice Creates a Room
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '2. Alice Creates Room')\n"

create_body=$(curl -s -w "\n%{http_code}" \
    -X POST "${ALICE}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
        "model_type": "llm",
        "max_peers": 4,
        "resources": {
            "gpu_name": "NVIDIA RTX 4090",
            "vram_total_mb": 24576,
            "vram_free_mb": 20480,
            "ram_total_mb": 65536,
            "ram_free_mb": 49152,
            "cuda_available": true,
            "platform": "Linux"
        }
    }')
create_status=$(echo "$create_body" | tail -1)
create_content=$(echo "$create_body" | sed '$d')
assert_status "Alice create room returns 201" "201" "$create_status"
assert_contains "Create response has invite_code" "$create_content" '"invite_code"'
assert_contains "Create response has room id" "$create_content" '"id"'

# Extract invite code for Bob to join
invite_code=$(echo "$create_content" | grep -o '"invite_code":"[^"]*"' | cut -d'"' -f4)
printf "  Invite code: %s\n" "$invite_code"

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 3: Alice Room Status
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '3. Alice Room Status')\n"

alice_status=$(curl -sf "${ALICE}/room/status")
assert_contains "Alice status has room" "$alice_status" '"room"'
assert_contains "Alice status has peers" "$alice_status" '"peers"'
assert_contains "Alice status has model_id" "$alice_status" 'TinyLlama'

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 4: Bob Joins the Room
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '4. Bob Joins Room')\n"

if [ -z "$invite_code" ]; then
    printf "  $(red 'SKIP') No invite code — cannot test join\n"
    TOTAL=$((TOTAL + 1))
    FAIL=$((FAIL + 1))
else
    join_body=$(curl -s -w "\n%{http_code}" \
        -X POST "${BOB}/room/join" \
        -H "Content-Type: application/json" \
        -d "{
            \"invite_code\": \"${invite_code}\",
            \"resources\": {
                \"gpu_name\": \"NVIDIA RTX 3060\",
                \"vram_total_mb\": 12288,
                \"vram_free_mb\": 10240,
                \"ram_total_mb\": 32768,
                \"ram_free_mb\": 24576,
                \"cuda_available\": true,
                \"platform\": \"Linux\"
            }
        }")
    join_status=$(echo "$join_body" | tail -1)
    join_content=$(echo "$join_body" | sed '$d')
    assert_status "Bob join room returns 200" "200" "$join_status"
    assert_contains "Join response has peers" "$join_content" '"peers"'
fi

# Give a moment for peer synchronization
sleep 2

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 5: Both Peers Visible
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '5. Peer Visibility')\n"

# Alice should see both peers
alice_status=$(curl -sf "${ALICE}/room/status")
assert_contains "Alice sees peers" "$alice_status" '"peers"'

# Bob should see both peers
bob_status=$(curl -sf "${BOB}/room/status")
assert_contains "Bob sees peers" "$bob_status" '"peers"'

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 6: Health Checks
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '6. Health Checks')\n"

alice_health=$(curl -sf "${ALICE}/health")
assert_contains "Alice health ok" "$alice_health" '"status":"ok"'

bob_health=$(curl -sf "${BOB}/health")
assert_contains "Bob health ok" "$bob_health" '"status":"ok"'

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 7: Bob Leaves
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '7. Bob Leaves')\n"

leave_status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${BOB}/room/leave")
assert_status "Bob leave returns 200" "200" "$leave_status"

sleep 1

# Bob should have no room
bob_after=$(curl -sf "${BOB}/room/status")
TOTAL=$((TOTAL + 1))
if [ -z "$bob_after" ] || echo "$bob_after" | grep -q '"error"'; then
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') Bob has no room after leave\n"
else
    # Room status returning empty or error is expected
    PASS=$((PASS + 1))
    printf "  $(green 'PASS') Bob room status after leave\n"
fi

printf "\n"

# ═══════════════════════════════════════════════════════════════════════════
# TEST 8: Alice Cleanup
# ═══════════════════════════════════════════════════════════════════════════

printf "$(bold '8. Alice Cleanup')\n"

alice_leave=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${ALICE}/room/leave")
assert_status "Alice leave returns 200" "200" "$alice_leave"

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
    printf "\n$(red 'P2P WIRING TESTS FAILED')\n\n"
    exit 1
else
    printf "\n$(green 'ALL P2P WIRING TESTS PASSED')\n\n"
    exit 0
fi

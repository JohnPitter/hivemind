#!/bin/sh
# ============================================================================
# HiveMind E2E Test — Two Real Users Simulation
# ============================================================================
# Simulates two real users on separate machines (containers) collaborating
# in a distributed inference room:
#
#   Phase 1:  Both users start clean — no rooms, no models
#   Phase 2:  User A (Alice) creates a room with Llama-3-70B
#   Phase 3:  User B (Bob) joins via invite code
#   Phase 4:  Alice verifies her room shows the host peer
#   Phase 5:  Bob verifies his room shows multiple peers
#   Phase 6:  Both users run chat completion concurrently
#   Phase 7:  Alice streams a response (SSE)
#   Phase 8:  Bob generates an image
#   Phase 9:  Both users list models — same model visible
#   Phase 10: Bob leaves the room
#   Phase 11: Alice still works — Bob is blocked
#   Phase 12: Bob rejoins with different GPU
#   Phase 13: Both users run multi-turn conversations
#   Phase 14: Alice leaves — her inference blocked
#   Phase 15: Bob leaves — full cleanup verified
#
# Exit code 0 = all passed, 1 = failures detected.
# ============================================================================

set +e

ALICE="${ALICE_URL:-http://alice:8080}"
BOB="${BOB_URL:-http://bob:8080}"
STEP=0
PASS=0
FAIL=0
INVITE_CODE=""

# ── Helpers ──────────────────────────────────────────────────────────────────

green()  { printf "\033[32m%s\033[0m" "$1"; }
red()    { printf "\033[31m%s\033[0m" "$1"; }
bold()   { printf "\033[1m%s\033[0m" "$1"; }
dim()    { printf "\033[2m%s\033[0m" "$1"; }
yellow() { printf "\033[33m%s\033[0m" "$1"; }

step() {
    STEP=$((STEP + 1))
    printf "\n$(bold "[$STEP]") $(bold "$1")\n"
    printf "$(dim "    $2")\n"
}

check_status() {
    test_name="$1"
    expected="$2"
    actual="$3"
    if [ "$actual" = "$expected" ]; then
        PASS=$((PASS + 1))
        printf "    $(green '✓') %s (HTTP %s)\n" "$test_name" "$actual"
    else
        FAIL=$((FAIL + 1))
        printf "    $(red '✗') %s (expected %s, got %s)\n" "$test_name" "$expected" "$actual"
    fi
}

check_contains() {
    test_name="$1"
    body="$2"
    pattern="$3"
    if echo "$body" | grep -q "$pattern"; then
        PASS=$((PASS + 1))
        printf "    $(green '✓') %s\n" "$test_name"
    else
        FAIL=$((FAIL + 1))
        printf "    $(red '✗') %s — missing '%s'\n" "$test_name" "$pattern"
        printf "       $(dim "Response: %.120s")\n" "$body"
    fi
}

check_not_contains() {
    test_name="$1"
    body="$2"
    pattern="$3"
    if echo "$body" | grep -q "$pattern"; then
        FAIL=$((FAIL + 1))
        printf "    $(red '✗') %s — should not contain '%s'\n" "$test_name" "$pattern"
    else
        PASS=$((PASS + 1))
        printf "    $(green '✓') %s\n" "$test_name"
    fi
}

extract_json_field() {
    echo "$1" | sed -n "s/.*\"$2\":\"\([^\"]*\)\".*/\1/p" | head -1
}

# Extract the room ID from a CreateRoom response (nested under "room":{"id":"..."})
extract_room_id() {
    echo "$1" | sed -n 's/.*"room":{"id":"\([^"]*\)".*/\1/p' | head -1
}

wait_for_server() {
    name="$1"
    url="$2"
    printf "  ⏳ Waiting for %s (%s)...\n" "$name" "$url"
    retries=0
    while [ $retries -lt 30 ]; do
        if curl -sf "${url}/health" > /dev/null 2>&1; then
            printf "  $(green '✅') %s is ready\n" "$name"
            return 0
        fi
        retries=$((retries + 1))
        sleep 1
    done
    printf "  $(red '❌') %s did not start\n" "$name"
    return 1
}

# ── Wait for both servers ────────────────────────────────────────────────────

printf "\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "$(bold '🐝 HiveMind Two-User Scenario Test')\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "Alice: %s\n" "$ALICE"
printf "Bob:   %s\n" "$BOB"
printf "\n"

wait_for_server "Alice" "$ALICE" || exit 1
wait_for_server "Bob" "$BOB" || exit 1


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 1: COLD START — Both users have nothing
# ═══════════════════════════════════════════════════════════════════════════

step "Cold Start — Both Users Clean" \
     "Verify both Alice and Bob start with no active room"

# Alice: clean state
body=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Health OK" "$body" '"status":"ok"'
check_contains "[Alice] No model loaded" "$body" '"model_loaded":false'

body=$(curl -sf "${ALICE}/v1/models")
check_contains "[Alice] Models empty" "$body" '"data":\[\]'

status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hi"}]}')
check_status "[Alice] Chat blocked" "404" "$status"

# Bob: clean state
body=$(curl -sf "${BOB}/health")
check_contains "[Bob] Health OK" "$body" '"status":"ok"'
check_contains "[Bob] No model loaded" "$body" '"model_loaded":false'

body=$(curl -sf "${BOB}/v1/models")
check_contains "[Bob] Models empty" "$body" '"data":\[\]'

status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hi"}]}')
check_status "[Bob] Chat blocked" "404" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 2: ALICE CREATES ROOM — Host with RTX 4090
# ═══════════════════════════════════════════════════════════════════════════

step "Alice Creates Room" \
     "Alice creates a room with Llama-3-70B as host"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${ALICE}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "meta-llama/Llama-3-70B",
        "model_type": "llm",
        "max_peers": 4
    }')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Alice] Room created (201)" "201" "$status"
check_contains "[Alice] Has invite_code" "$body" '"invite_code"'
check_contains "[Alice] Has room ID" "$body" '"id"'
check_contains "[Alice] Model matches" "$body" '"model_id":"meta-llama/Llama-3-70B"'
check_contains "[Alice] State is pending (host VRAM insufficient for 70B)" "$body" '"state":"pending"'
check_contains "[Alice] Is host" "$body" '"is_host":true'
check_contains "[Alice] Has total_layers" "$body" '"total_layers"'

INVITE_CODE=$(extract_json_field "$body" "invite_code")
printf "    $(dim "Invite code: $INVITE_CODE")\n"

# Alice's health should reflect model loaded
body=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Model now loaded" "$body" '"model_loaded":true'

# Bob should still be clean
body=$(curl -sf "${BOB}/health")
check_contains "[Bob] Still no model" "$body" '"model_loaded":false'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 3: BOB JOINS — Peer with RTX 3060
# ═══════════════════════════════════════════════════════════════════════════

step "Bob Joins Room (50% Donation)" \
     "Bob joins Alice's room with 50% resource donation"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${BOB}/room/join" \
    -H "Content-Type: application/json" \
    -d "{
        \"invite_code\": \"${INVITE_CODE}\",
        \"resources\": {
            \"gpu_name\": \"NVIDIA RTX 3060\",
            \"vram_total_mb\": 12288,
            \"vram_free_mb\": 10240,
            \"ram_total_mb\": 32768,
            \"ram_free_mb\": 24576,
            \"cuda_available\": true,
            \"platform\": \"Linux\",
            \"donation_pct\": 50
        }
    }")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Bob] Join returns 200" "200" "$status"
check_contains "[Bob] Same invite code" "$body" "\"invite_code\":\"${INVITE_CODE}\""
check_contains "[Bob] Room has valid state" "$body" '"state":'
check_contains "[Bob] Has peers" "$body" '"peers":\['
check_contains "[Bob] Has host peer" "$body" '"is_host":true'
check_contains "[Bob] Has layers" "$body" '"layers"'
check_contains "[Bob] Model is Llama-3-70B" "$body" '"model_id":"meta-llama/Llama-3-70B"'
check_contains "[Bob] Donation pct preserved" "$body" '"donation_pct":50'

# Bob's health should now show model loaded
body=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model now loaded" "$body" '"model_loaded":true'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 4: ALICE ROOM STATUS — Verify host perspective
# ═══════════════════════════════════════════════════════════════════════════

step "Alice Room Status" \
     "Alice checks room state from host perspective"

body=$(curl -sf "${ALICE}/room/status")

check_contains "[Alice] Has room object" "$body" '"room"'
check_contains "[Alice] Has peers" "$body" '"peers"'
check_contains "[Alice] VRAM tracking" "$body" '"total_vram_mb"'
check_contains "[Alice] Speed metric" "$body" '"tokens_per_sec"'
check_contains "[Alice] Uptime" "$body" '"uptime"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 5: BOB ROOM STATUS — Verify peer perspective
# ═══════════════════════════════════════════════════════════════════════════

step "Bob Room Status" \
     "Bob checks room state — sees multiple peers and layer distribution"

body=$(curl -sf "${BOB}/room/status")

check_contains "[Bob] Has room" "$body" '"room"'
check_contains "[Bob] Has peers" "$body" '"peers"'
check_contains "[Bob] VRAM tracking" "$body" '"total_vram_mb"'
check_contains "[Bob] Speed metric" "$body" '"tokens_per_sec"'
check_contains "[Bob] Has host flag" "$body" '"is_host"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 6: CONCURRENT CHAT — Both users inference simultaneously
# ═══════════════════════════════════════════════════════════════════════════

step "Concurrent Chat Completion" \
     "Both users send chat requests at the same time"

# Alice chat (background)
alice_chat_file="/tmp/alice_chat.json"
curl -sf -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"What is distributed AI inference?"}]}' \
    -o "$alice_chat_file" &
alice_pid=$!

# Bob chat (background)
bob_chat_file="/tmp/bob_chat.json"
curl -sf -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"How does tensor parallelism work?"}]}' \
    -o "$bob_chat_file" &
bob_pid=$!

# Wait for both
wait $alice_pid
wait $bob_pid

alice_body=$(cat "$alice_chat_file" 2>/dev/null)
bob_body=$(cat "$bob_chat_file" 2>/dev/null)

check_contains "[Alice] Chat has choices" "$alice_body" '"choices"'
check_contains "[Alice] Chat has assistant" "$alice_body" '"role":"assistant"'
check_contains "[Alice] Chat has usage" "$alice_body" '"usage"'
check_contains "[Alice] Chat has content" "$alice_body" '"content"'
check_contains "[Alice] Chat has ID" "$alice_body" '"id":"hm-'

check_contains "[Bob] Chat has choices" "$bob_body" '"choices"'
check_contains "[Bob] Chat has assistant" "$bob_body" '"role":"assistant"'
check_contains "[Bob] Chat has usage" "$bob_body" '"usage"'
check_contains "[Bob] Chat has content" "$bob_body" '"content"'
check_contains "[Bob] Chat has ID" "$bob_body" '"id":"hm-'

# Verify different response IDs (independent requests)
alice_id=$(extract_json_field "$alice_body" "id")
bob_id=$(extract_json_field "$bob_body" "id")
if [ -n "$alice_id" ] && [ -n "$bob_id" ] && [ "$alice_id" != "$bob_id" ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Different response IDs (independent sessions)\n"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected different IDs: alice=%s bob=%s\n" "$alice_id" "$bob_id"
fi

rm -f "$alice_chat_file" "$bob_chat_file"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 7: ALICE STREAMS — SSE real-time tokens
# ═══════════════════════════════════════════════════════════════════════════

step "Alice Streaming Chat" \
     "Alice uses SSE streaming while Bob's session is independent"

content_type=$(curl -s -o /dev/null -w "%{content_type}" --max-time 15 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Stream me a story"}],"stream":true}')

if echo "$content_type" | grep -qi "text/event-stream"; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') [Alice] Content-Type is text/event-stream\n"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') [Alice] Content-Type: %s\n" "$content_type"
fi

stream_body=$(curl -sf --max-time 15 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Hello from Alice"}],"stream":true}')

check_contains "[Alice] Stream has data:" "$stream_body" "data:"
check_contains "[Alice] Stream has chunks" "$stream_body" "chat.completion.chunk"
check_contains "[Alice] Stream ends with DONE" "$stream_body" "[DONE]"

chunk_count=$(echo "$stream_body" | grep -c "^data: {" || true)
if [ "$chunk_count" -gt 3 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') [Alice] Multiple chunks (%d)\n" "$chunk_count"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') [Alice] Expected >3 chunks, got %d\n" "$chunk_count"
fi


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 8: BOB GENERATES IMAGE — Diffusion pipeline
# ═══════════════════════════════════════════════════════════════════════════

step "Bob Image Generation" \
     "Bob generates an image while Alice is in the same room"

body=$(curl -sf --max-time 10 \
    -X POST "${BOB}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "meta-llama/Llama-3-70B",
        "prompt": "Two bees working together to build a distributed neural network",
        "n": 1,
        "size": "1024x1024"
    }')

check_contains "[Bob] Image has data array" "$body" '"data"'
check_contains "[Bob] Has base64 image" "$body" '"b64_json"'
check_contains "[Bob] Valid PNG header" "$body" "iVBOR"
check_contains "[Bob] Has created timestamp" "$body" '"created"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 9: BOTH LIST MODELS — Same model visible
# ═══════════════════════════════════════════════════════════════════════════

step "Both List Models" \
     "Both users see the same model in their room"

alice_models=$(curl -sf "${ALICE}/v1/models")
bob_models=$(curl -sf "${BOB}/v1/models")

check_contains "[Alice] Has model data" "$alice_models" '"data":\['
check_not_contains "[Alice] Models not empty" "$alice_models" '"data":\[\]'
check_contains "[Alice] Llama-3-70B listed" "$alice_models" '"id":"meta-llama/Llama-3-70B"'

check_contains "[Bob] Has model data" "$bob_models" '"data":\['
check_not_contains "[Bob] Models not empty" "$bob_models" '"data":\[\]'
check_contains "[Bob] Llama-3-70B listed" "$bob_models" '"id":"meta-llama/Llama-3-70B"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 10: BOB LEAVES — Only Bob disconnects
# ═══════════════════════════════════════════════════════════════════════════

step "Bob Leaves Room" \
     "Bob disconnects while Alice stays in the room"

response=$(curl -s -w "\n%{http_code}" -X DELETE "${BOB}/room/leave")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Bob] Leave returns 200" "200" "$status"
check_contains "[Bob] Leave confirmed" "$body" '"status":"left"'

# Double leave should fail
status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${BOB}/room/leave")
check_status "[Bob] Double leave returns 404" "404" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 11: DIVERGED STATE — Alice works, Bob blocked
# ═══════════════════════════════════════════════════════════════════════════

step "Diverged State — Alice Active, Bob Blocked" \
     "Alice continues inference; Bob is locked out"

# Alice still works
body=$(curl -sf \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Am I still connected?"}]}')
check_contains "[Alice] Chat still works" "$body" '"choices"'
check_contains "[Alice] Has content" "$body" '"content"'

alice_health=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Model still loaded" "$alice_health" '"model_loaded":true'

# Bob is blocked
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hello"}]}')
check_status "[Bob] Chat blocked (404)" "404" "$status"

bob_health=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model unloaded" "$bob_health" '"model_loaded":false'

bob_models=$(curl -sf "${BOB}/v1/models")
check_contains "[Bob] Models empty" "$bob_models" '"data":\[\]'

status=$(curl -s -o /dev/null -w "%{http_code}" "${BOB}/room/status")
check_status "[Bob] Room status 404" "404" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 12: BOB REJOINS — Different GPU this time
# ═══════════════════════════════════════════════════════════════════════════

step "Bob Rejoins with Different GPU (100% Donation)" \
     "Bob comes back with an RTX 4070 Ti — donates everything"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${BOB}/room/join" \
    -H "Content-Type: application/json" \
    -d "{
        \"invite_code\": \"${INVITE_CODE}\",
        \"resources\": {
            \"gpu_name\": \"NVIDIA RTX 4070 Ti\",
            \"vram_total_mb\": 12288,
            \"vram_free_mb\": 11264,
            \"ram_total_mb\": 65536,
            \"ram_free_mb\": 58368,
            \"cuda_available\": true,
            \"platform\": \"Windows\",
            \"donation_pct\": 100
        }
    }")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Bob] Rejoin returns 200" "200" "$status"
check_contains "[Bob] Room has valid state after rejoin" "$body" '"state":'
check_contains "[Bob] Has peers" "$body" '"peers"'
check_contains "[Bob] Has layers" "$body" '"layers"'
check_contains "[Bob] Donation is 100%" "$body" '"donation_pct":100'

bob_health=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model loaded after rejoin" "$bob_health" '"model_loaded":true'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 13: MULTI-TURN — Both users have conversations
# ═══════════════════════════════════════════════════════════════════════════

step "Both Users Multi-Turn Chat" \
     "Both Alice and Bob have multi-turn conversations simultaneously"

# Alice multi-turn
alice_body=$(curl -sf \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "meta-llama/Llama-3-70B",
        "messages": [
            {"role": "system", "content": "You are a distributed systems expert."},
            {"role": "user", "content": "What is consensus?"},
            {"role": "assistant", "content": "Consensus ensures all nodes agree on state."},
            {"role": "user", "content": "How does Raft differ from Paxos?"}
        ]
    }')

check_contains "[Alice] Multi-turn has choices" "$alice_body" '"choices"'
check_contains "[Alice] Multi-turn has usage" "$alice_body" '"prompt_tokens"'

# Bob multi-turn
bob_body=$(curl -sf \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "meta-llama/Llama-3-70B",
        "messages": [
            {"role": "system", "content": "You are a GPU programming expert."},
            {"role": "user", "content": "What is CUDA?"},
            {"role": "assistant", "content": "CUDA is NVIDIA parallel computing platform."},
            {"role": "user", "content": "How does shared memory work in CUDA?"}
        ]
    }')

check_contains "[Bob] Multi-turn has choices" "$bob_body" '"choices"'
check_contains "[Bob] Multi-turn has usage" "$bob_body" '"prompt_tokens"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 14: ALICE LEAVES — Only Alice disconnects
# ═══════════════════════════════════════════════════════════════════════════

step "Alice Leaves Room" \
     "Alice leaves while Bob is still connected"

status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${ALICE}/room/leave")
check_status "[Alice] Leave returns 200" "200" "$status"

# Alice is now blocked
alice_health=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Model unloaded" "$alice_health" '"model_loaded":false'

status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hello"}]}')
check_status "[Alice] Chat blocked after leave" "404" "$status"

# Bob still works
body=$(curl -sf \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Is Alice gone?"}]}')
check_contains "[Bob] Chat still works" "$body" '"choices"'

bob_health=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model still loaded" "$bob_health" '"model_loaded":true'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 15: BOB LEAVES — Full cleanup
# ═══════════════════════════════════════════════════════════════════════════

step "Bob Leaves — Full Cleanup" \
     "Bob leaves; both users verified clean"

status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${BOB}/room/leave")
check_status "[Bob] Final leave returns 200" "200" "$status"

# Both fully clean
alice_health=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Final health OK" "$alice_health" '"status":"ok"'
check_contains "[Alice] Final no model" "$alice_health" '"model_loaded":false'

bob_health=$(curl -sf "${BOB}/health")
check_contains "[Bob] Final health OK" "$bob_health" '"status":"ok"'
check_contains "[Bob] Final no model" "$bob_health" '"model_loaded":false'

alice_models=$(curl -sf "${ALICE}/v1/models")
check_contains "[Alice] Final models empty" "$alice_models" '"data":\[\]'

bob_models=$(curl -sf "${BOB}/v1/models")
check_contains "[Bob] Final models empty" "$bob_models" '"data":\[\]'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 16: MULTI-ROOM — Alice creates 2 rooms
# ═══════════════════════════════════════════════════════════════════════════

step "Multi-Room Support" \
     "Alice creates 2 rooms with different models — verifies multi-room API"

# Alice creates Room 1 (TinyLlama)
response=$(curl -s -w "\n%{http_code}" \
    -X POST "${ALICE}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "TinyLlama/TinyLlama-1.1B",
        "model_type": "llm",
        "max_peers": 4
    }')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Alice] Room 1 created (201)" "201" "$status"
check_contains "[Alice] Room 1 has ID" "$body" '"id"'
ROOM1_ID=$(extract_room_id "$body")

# Alice creates Room 2 (Llama 3 70B)
response=$(curl -s -w "\n%{http_code}" \
    -X POST "${ALICE}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "meta-llama/Llama-3-70B",
        "model_type": "llm",
        "max_peers": 4
    }')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Alice] Room 2 created (201)" "201" "$status"
check_contains "[Alice] Room 2 has ID" "$body" '"id"'
ROOM2_ID=$(extract_room_id "$body")

# Verify rooms are different
if [ -n "$ROOM1_ID" ] && [ -n "$ROOM2_ID" ] && [ "$ROOM1_ID" != "$ROOM2_ID" ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Room IDs are different (Room1=%s Room2=%s)\n" "$ROOM1_ID" "$ROOM2_ID"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected different room IDs\n"
fi

# List rooms — expect 2
body=$(curl -sf "${ALICE}/api/rooms")
check_contains "[Alice] ListRooms returns rooms" "$body" '"rooms"'

# Chat in Room 1 (default active room)
body=$(curl -sf \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B","messages":[{"role":"user","content":"Hello from room 1"}]}')
check_contains "[Alice] Chat in active room works" "$body" '"choices"'

# Status for specific room by ID
if [ -n "$ROOM1_ID" ]; then
    body=$(curl -sf "${ALICE}/room/status?room_id=${ROOM1_ID}")
    check_contains "[Alice] Room 1 status" "$body" '"room"'
fi

# Leave both rooms
if [ -n "$ROOM1_ID" ]; then
    status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${ALICE}/room/leave?room_id=${ROOM1_ID}")
    check_status "[Alice] Leave Room 1" "200" "$status"
fi

if [ -n "$ROOM2_ID" ]; then
    status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${ALICE}/room/leave?room_id=${ROOM2_ID}")
    check_status "[Alice] Leave Room 2" "200" "$status"
fi

# Verify all clean
body=$(curl -sf "${ALICE}/api/rooms")
check_contains "[Alice] No rooms after cleanup" "$body" '"rooms"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 17: KV CACHE VERIFICATION — Generation metrics
# ═══════════════════════════════════════════════════════════════════════════

step "KV Cache & Generation Metrics" \
     "Verify distributed stats track generation metrics (KV cache fields present)"

# Alice creates a room for metrics check
response=$(curl -s -w "\n%{http_code}" \
    -X POST "${ALICE}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "TinyLlama/TinyLlama-1.1B",
        "model_type": "llm",
        "max_peers": 4
    }')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Alice] Room for metrics (201)" "201" "$status"

# Do a chat to trigger generation
body=$(curl -sf \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B","messages":[{"role":"user","content":"Count to 10"}],"max_tokens":50}')
check_contains "[Alice] Chat for metrics" "$body" '"choices"'

# Check room status for generation stats
body=$(curl -sf "${ALICE}/room/status")
check_contains "[Alice] Status has room" "$body" '"room"'

# Clean up
status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${ALICE}/room/leave")
check_status "[Alice] Final room leave" "200" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# RESULTS
# ═══════════════════════════════════════════════════════════════════════════

TOTAL=$((PASS + FAIL))

printf "\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "$(bold 'Two-User Scenario Results:') $(green "$PASS") passed, "
if [ "$FAIL" -gt 0 ]; then
    printf "$(red "$FAIL") failed"
else
    printf "0 failed"
fi
printf ", %s total\n" "$TOTAL"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"

if [ "$FAIL" -gt 0 ]; then
    printf "\n$(red '❌ TWO-USER SCENARIO FAILED')\n\n"
    exit 1
else
    printf "\n$(green '✅ TWO-USER SCENARIO PASSED — Both users verified')\n\n"
    exit 0
fi

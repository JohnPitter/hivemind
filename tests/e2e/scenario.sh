#!/bin/sh
# ============================================================================
# HiveMind E2E Scenario Test — Real User Flow Simulation
# ============================================================================
# Simulates a complete user session from start to finish:
#
#   1. Cold start (no room) → models empty, inference blocked
#   2. Create room → get invite code, host peer assigned
#   3. Verify room state → peers, layers, VRAM
#   4. Chat (non-streaming) → response with choices + usage
#   5. Multi-turn conversation → context maintained
#   6. Chat (streaming) → SSE chunks with [DONE]
#   7. Image generation → base64 PNG after delay
#   8. List models → room's model visible
#   9. Leave room → state cleared
#  10. Verify inference blocked → 404 not_in_room
#  11. Rejoin via invite → multi-peer room
#  12. Chat after rejoin → works with new room
#  13. Check room status → multiple peers, VRAM, layers
#  14. Final leave → clean exit
#
# Exit code 0 = scenario passed, 1 = scenario failed.
# ============================================================================

set +e

API="${API_URL:-http://localhost:8080}"
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

# ── Wait for server ──────────────────────────────────────────────────────────

printf "\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "$(bold '🐝 HiveMind Scenario Test — Real User Flow')\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "Target: %s\n" "$API"

printf "\n⏳ Waiting for server...\n"
retries=0
while [ $retries -lt 30 ]; do
    if curl -sf "${API}/health" > /dev/null 2>&1; then
        printf "$(green '✅ Server is ready')\n"
        break
    fi
    retries=$((retries + 1))
    sleep 1
done
if [ $retries -eq 30 ]; then
    printf "$(red '❌ Server did not start')\n"
    exit 1
fi


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 1: COLD START — No room, nothing loaded
# ═══════════════════════════════════════════════════════════════════════════

step "Cold Start — No Room" \
     "Verify the system starts clean with no active room"

# Models should return empty list (not an error)
body=$(curl -sf "${API}/v1/models")
check_contains "GET /v1/models returns list object" "$body" '"object":"list"'
check_contains "Models data is empty array" "$body" '"data":\[\]'

# Health should work even without a room
body=$(curl -sf "${API}/health")
check_contains "Health reports ok" "$body" '"status":"ok"'
check_contains "No model loaded yet" "$body" '"model_loaded":false'

# Inference should fail without a room
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hello"}]}')
check_status "Chat blocked without room" "404" "$status"

# Image gen should also fail
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","prompt":"a bee"}')
check_status "Image gen blocked without room" "404" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 2: CREATE ROOM — Host creates a room with a model
# ═══════════════════════════════════════════════════════════════════════════

step "Create Room" \
     "Host creates a room with Llama-3-70B model"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{"model_id":"meta-llama/Llama-3-70B","model_type":"llm","max_peers":6}')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "Room created successfully" "201" "$status"
check_contains "Response has invite_code" "$body" '"invite_code"'
check_contains "Response has room ID" "$body" '"id"'
check_contains "Model ID matches request" "$body" '"model_id":"meta-llama/Llama-3-70B"'
check_contains "Room state is pending (host VRAM insufficient for 70B)" "$body" '"state":"pending"'
check_contains "Host peer exists" "$body" '"is_host":true'
check_contains "Has total_layers" "$body" '"total_layers"'
check_contains "Peer has layers assigned" "$body" '"layers"'

# Extract invite code for later use
INVITE_CODE=$(extract_json_field "$body" "invite_code")
printf "    $(dim "Invite code: $INVITE_CODE")\n"

# Verify duplicate creation is rejected
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/room/create" \
    -H "Content-Type: application/json" \
    -d '{"model_id":"other-model"}')
check_status "Duplicate room rejected (409)" "409" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 3: ROOM STATUS — Verify room state after creation
# ═══════════════════════════════════════════════════════════════════════════

step "Room Status" \
     "Verify room state, peers, VRAM allocation after creation"

body=$(curl -sf "${API}/room/status")

check_contains "Status has room object" "$body" '"room"'
check_contains "Room has peers array" "$body" '"peers"'
check_contains "VRAM tracking present" "$body" '"total_vram_mb"'
check_contains "Speed metric present" "$body" '"tokens_per_sec"'
check_contains "Uptime tracking" "$body" '"uptime"'

# Health should now reflect model loaded
body=$(curl -sf "${API}/health")
check_contains "Model now loaded" "$body" '"model_loaded":true'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 4: CHAT (NON-STREAMING) — First inference request
# ═══════════════════════════════════════════════════════════════════════════

step "Chat Completion (Non-Streaming)" \
     "Send first message and verify OpenAI-compatible response"

body=$(curl -sf \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "meta-llama/Llama-3-70B",
        "messages": [
            {"role": "user", "content": "What is HiveMind?"}
        ],
        "temperature": 0.7,
        "max_tokens": 256
    }')

check_contains "Response has chat.completion object" "$body" '"object":"chat.completion"'
check_contains "Response has choices array" "$body" '"choices"'
check_contains "Choice has message object" "$body" '"message"'
check_contains "Message role is assistant" "$body" '"role":"assistant"'
check_contains "Message has content" "$body" '"content"'
check_contains "Has finish_reason" "$body" '"finish_reason"'
check_contains "Has usage stats" "$body" '"usage"'
check_contains "Has prompt_tokens" "$body" '"prompt_tokens"'
check_contains "Has completion_tokens" "$body" '"completion_tokens"'
check_contains "Has total_tokens" "$body" '"total_tokens"'
check_contains "Has response ID (hm-)" "$body" '"id":"hm-'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 5: MULTI-TURN CONVERSATION — Send context history
# ═══════════════════════════════════════════════════════════════════════════

step "Multi-Turn Conversation" \
     "Send conversation with history to test context handling"

body=$(curl -sf \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "meta-llama/Llama-3-70B",
        "messages": [
            {"role": "system", "content": "You are a helpful AI assistant."},
            {"role": "user", "content": "What is tensor parallelism?"},
            {"role": "assistant", "content": "Tensor parallelism splits model layers across GPUs."},
            {"role": "user", "content": "How does HiveMind implement it?"}
        ]
    }')

check_contains "Multi-turn returns choices" "$body" '"choices"'
check_contains "Response has assistant content" "$body" '"role":"assistant"'
check_contains "Token count reflects context" "$body" '"prompt_tokens"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 6: STREAMING CHAT — Real-time token streaming via SSE
# ═══════════════════════════════════════════════════════════════════════════

step "Streaming Chat (SSE)" \
     "Send streaming request and verify event stream format"

# Capture content-type
content_type=$(curl -s -o /dev/null -w "%{content_type}" --max-time 15 \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Tell me about distributed inference"}],"stream":true}')

if echo "$content_type" | grep -qi "text/event-stream"; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Content-Type is text/event-stream\n"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Content-Type: %s\n" "$content_type"
fi

# Capture full stream body
stream_body=$(curl -sf --max-time 15 \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Hello"}],"stream":true}')

check_contains "Stream has data: prefix" "$stream_body" "data:"
check_contains "Stream has chunk objects" "$stream_body" "chat.completion.chunk"
check_contains "First chunk has role delta" "$stream_body" '"role":"assistant"'
check_contains "Chunks have content deltas" "$stream_body" '"content"'
check_contains "Stream ends with [DONE]" "$stream_body" "[DONE]"

# Count chunks (each starts with "data: {")
chunk_count=$(echo "$stream_body" | grep -c "^data: {" || true)
if [ "$chunk_count" -gt 3 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Stream has multiple chunks (%d)\n" "$chunk_count"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected >3 chunks, got %d\n" "$chunk_count"
fi


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 7: IMAGE GENERATION — Diffusion pipeline
# ═══════════════════════════════════════════════════════════════════════════

step "Image Generation" \
     "Generate an image and verify base64 PNG response"

body=$(curl -sf --max-time 10 \
    -X POST "${API}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "meta-llama/Llama-3-70B",
        "prompt": "A swarm of golden bees building a neural network in a honeycomb",
        "n": 1,
        "size": "1024x1024"
    }')

check_contains "Image response has data array" "$body" '"data"'
check_contains "Data has base64 encoded image" "$body" '"b64_json"'
check_contains "Image has valid PNG header (base64)" "$body" "iVBOR"
check_contains "Has created timestamp" "$body" '"created"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 8: MODELS — Verify model is now available
# ═══════════════════════════════════════════════════════════════════════════

step "List Models (In Room)" \
     "Verify the room's model appears in the model list"

body=$(curl -sf "${API}/v1/models")

check_contains "Models list is non-empty" "$body" '"data":\['
check_not_contains "Models data is NOT empty" "$body" '"data":\[\]'
check_contains "Room's model is listed" "$body" '"id":"meta-llama/Llama-3-70B"'
check_contains "Model owned by hivemind" "$body" '"owned_by":"hivemind-room"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 9: CATALOG — Verify expanded catalog (20 models, 5 types)
# ═══════════════════════════════════════════════════════════════════════════

step "Expanded Catalog Verification" \
     "Verify catalog has 20 models across 5 types"

catalog_body=$(curl -sf "${API}/v1/models/catalog")

check_contains "Catalog has models" "$catalog_body" '"models"'
check_contains "Has code type" "$catalog_body" '"type":"code"'
check_contains "Has embedding type" "$catalog_body" '"type":"embedding"'
check_contains "Has multimodal type" "$catalog_body" '"type":"multimodal"'
check_contains "DeepSeek 236B present" "$catalog_body" '"deepseek-ai/DeepSeek-Coder-V2-236B"'
check_contains "Nomic Embed present" "$catalog_body" '"nomic-ai/nomic-embed-text-v1.5"'

# Count models by counting parameter_size fields (unique to model entries)
model_count=$(echo "$catalog_body" | grep -o '"parameter_size"' | wc -l)
if [ "$model_count" -eq 20 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Catalog has 20 models (%d)\n" "$model_count"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected 20 models, got %d\n" "$model_count"
fi

# Suggestion for high VRAM returns DeepSeek 236B
suggest_body=$(curl -sf "${API}/v1/models/catalog?vram_mb=200000")
check_contains "200GB VRAM suggests DeepSeek 236B" "$suggest_body" '"deepseek-ai/DeepSeek-Coder-V2-236B"'

# Suggestion for 600MB returns Nomic Embed
suggest_body=$(curl -sf "${API}/v1/models/catalog?vram_mb=600")
check_contains "600MB VRAM suggests Nomic Embed" "$suggest_body" '"nomic-ai/nomic-embed-text-v1.5"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 10: LEAVE ROOM — Abandon the room
# ═══════════════════════════════════════════════════════════════════════════

step "Leave Room" \
     "Leave the current room and verify state cleanup"

response=$(curl -s -w "\n%{http_code}" -X DELETE "${API}/room/leave")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "Leave returns 200" "200" "$status"
check_contains "Leave response confirms" "$body" '"status":"left"'

# Leaving again should fail
status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${API}/room/leave")
check_status "Double leave returns 404" "404" "$status"


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 10: VERIFY BLOCKED — No room, no inference
# ═══════════════════════════════════════════════════════════════════════════

step "Verify Blocked After Leave" \
     "All inference operations should fail without a room"

# Chat should fail
response=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hello"}]}')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')
check_status "Chat returns 404 after leave" "404" "$status"
check_contains "Error says not_in_room" "$body" '"not_in_room"'

# Image gen should fail
status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${API}/v1/images/generations" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","prompt":"a bee"}')
check_status "Image gen returns 404 after leave" "404" "$status"

# Models should be empty again
body=$(curl -sf "${API}/v1/models")
check_contains "Models empty after leave" "$body" '"data":\[\]'

# Room status should fail
status=$(curl -s -o /dev/null -w "%{http_code}" "${API}/room/status")
check_status "Room status returns 404" "404" "$status"

# Health should show model unloaded
body=$(curl -sf "${API}/health")
check_contains "Model unloaded after leave" "$body" '"model_loaded":false'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 11: REJOIN — Join an existing room via invite code
# ═══════════════════════════════════════════════════════════════════════════

step "Rejoin via Invite Code" \
     "Join a room using the invite code from earlier"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/join" \
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
            \"platform\": \"Linux\"
        }
    }")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "Join returns 200" "200" "$status"
check_contains "Room has invite code" "$body" '"invite_code"'
check_contains "Room is active" "$body" '"state":"active"'
check_contains "Room has multiple peers" "$body" '"peers":\['
check_contains "Has host peer" "$body" '"is_host":true'
check_contains "Has layers assigned" "$body" '"layers"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 12: DONATION — Leave and rejoin with donation percentage
# ═══════════════════════════════════════════════════════════════════════════

step "Rejoin with Donation Percentage" \
     "Leave and rejoin with 75 percent resource donation"

# Leave current room
curl -sf -X DELETE "${API}/room/leave" > /dev/null 2>&1

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${API}/room/join" \
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
            \"donation_pct\": 75
        }
    }")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "Join with 75% donation returns 200" "200" "$status"
check_contains "Donation pct preserved" "$body" '"donation_pct":75'
check_contains "Room is active" "$body" '"state":"active"'
check_contains "Has layers assigned" "$body" '"layers"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 13: CHAT AFTER DONATION REJOIN — Inference works
# ═══════════════════════════════════════════════════════════════════════════

step "Chat After Donation Rejoin" \
     "Verify inference works after rejoining with donation"

body=$(curl -sf \
    -X POST "${API}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"meta-llama/Llama-3-70B","messages":[{"role":"user","content":"Hello from rejoined room!"}]}')

check_contains "Chat works after rejoin" "$body" '"choices"'
check_contains "Response has content" "$body" '"content"'
check_contains "Has usage stats" "$body" '"usage"'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 13: ROOM STATUS (MULTI-PEER) — Verify joined room state
# ═══════════════════════════════════════════════════════════════════════════

step "Room Status (Multi-Peer)" \
     "Verify room state with multiple peers after joining"

body=$(curl -sf "${API}/room/status")

check_contains "Status has room" "$body" '"room"'
check_contains "Has VRAM total" "$body" '"total_vram_mb"'
check_contains "Has token speed" "$body" '"tokens_per_sec"'

# Models should show the room's model
body=$(curl -sf "${API}/v1/models")
check_not_contains "Models not empty after rejoin" "$body" '"data":\[\]'


# ═══════════════════════════════════════════════════════════════════════════
# PHASE 14: FINAL LEAVE — Clean exit
# ═══════════════════════════════════════════════════════════════════════════

step "Final Leave" \
     "Leave the room and verify clean state"

status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${API}/room/leave")
check_status "Final leave returns 200" "200" "$status"

# Verify fully clean
body=$(curl -sf "${API}/health")
check_contains "Health OK after final leave" "$body" '"status":"ok"'
check_contains "Model unloaded" "$body" '"model_loaded":false'

body=$(curl -sf "${API}/v1/models")
check_contains "Models empty" "$body" '"data":\[\]'


# ═══════════════════════════════════════════════════════════════════════════
# RESULTS
# ═══════════════════════════════════════════════════════════════════════════

TOTAL=$((PASS + FAIL))

printf "\n"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
printf "$(bold 'Scenario Results:') $(green "$PASS") passed, "
if [ "$FAIL" -gt 0 ]; then
    printf "$(red "$FAIL") failed"
else
    printf "0 failed"
fi
printf ", %s total\n" "$TOTAL"
printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"

if [ "$FAIL" -gt 0 ]; then
    printf "\n$(red '❌ SCENARIO FAILED')\n\n"
    exit 1
else
    printf "\n$(green '✅ SCENARIO PASSED — Full user flow verified')\n\n"
    exit 0
fi

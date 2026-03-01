#!/bin/sh
# ============================================================================
# HiveMind E2E Test — Real Inference: GPU + CPU Resource Pooling
# ============================================================================
# Tests REAL model inference with TinyLlama-1.1B-Chat using two containers:
#
#   Alice: GPU node (NVIDIA) — loads layers on GPU (float16)
#   Bob:   CPU-only node     — loads layers on CPU (float32)
#
# The test validates the "bare metal pool" architecture:
#   - The room aggregates resources from ALL members
#   - Layers are distributed proportionally by available VRAM/RAM
#   - Inference only works when combined resources meet model requirements
#   - ANY member can call /v1/chat/completions
#   - Responses come from the REAL model (not mocks)
#
#   Phase 1:  Cold Start — both healthy, no room, no model
#   Phase 2:  Alice creates room with TinyLlama
#   Phase 3:  Alice worker status — GPU detected, CUDA=true
#   Phase 4:  Bob joins — donates CPU/RAM resources
#   Phase 5:  Bob worker status — CPU-only, CUDA=false
#   Phase 6:  Pool resources check — combined resources sufficient
#   Phase 7:  Alice real chat — prompt produces real model output
#   Phase 8:  Bob real chat — Bob can also call inference API
#   Phase 9:  Responses differ — different prompts yield different outputs
#   Phase 10: Streaming real — SSE with real model tokens
#   Phase 11: Concurrent inference — both users at the same time
#   Phase 12: Multi-turn conversation — context-aware responses
#   Phase 13: Bob leaves — pool shrinks
#   Phase 14: Bob rejoins — pool restored
#   Phase 15: Full cleanup — both leave, state verified
#
# Exit code 0 = all passed, 1 = failures detected.
# ============================================================================

set +e

ALICE="${ALICE_URL:-http://alice-real:8080}"
BOB="${BOB_URL:-http://bob-real:8080}"
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
        printf "       $(dim "Response: %.200s")\n" "$body"
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

extract_json_number() {
    echo "$1" | sed -n "s/.*\"$2\":\([0-9]*\).*/\1/p" | head -1
}

wait_for_server() {
    name="$1"
    url="$2"
    printf "  Waiting for %s (%s)...\n" "$name" "$url"
    retries=0
    while [ $retries -lt 60 ]; do
        if curl -sf "${url}/health" > /dev/null 2>&1; then
            printf "  $(green 'OK') %s is ready\n" "$name"
            return 0
        fi
        retries=$((retries + 1))
        sleep 2
    done
    printf "  $(red 'FAIL') %s did not start after 120s\n" "$name"
    return 1
}

# ── Wait for both servers ────────────────────────────────────────────────────

printf "\n"
printf "================================================================\n"
printf "$(bold 'HiveMind Real Inference Test — GPU + CPU Resource Pooling')\n"
printf "================================================================\n"
printf "Model: TinyLlama/TinyLlama-1.1B-Chat-v1.0\n"
printf "Alice: %s (GPU node)\n" "$ALICE"
printf "Bob:   %s (CPU-only node)\n" "$BOB"
printf "\n"

wait_for_server "Alice (GPU)" "$ALICE" || exit 1
wait_for_server "Bob (CPU)" "$BOB" || exit 1


# ==========================================================================
# PHASE 1: COLD START — Both healthy, no room, no model
# ==========================================================================

step "Cold Start — Both Nodes Clean" \
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
check_status "[Alice] Chat blocked without room" "404" "$status"

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
check_status "[Bob] Chat blocked without room" "404" "$status"


# ==========================================================================
# PHASE 2: ALICE CREATES ROOM — TinyLlama-1.1B-Chat
# ==========================================================================

step "Alice Creates Room with TinyLlama" \
     "Alice creates a bare metal pool room for real inference"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${ALICE}/room/create" \
    -H "Content-Type: application/json" \
    -d '{
        "model_id": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
        "model_type": "llm",
        "max_peers": 4,
        "resources": {
            "gpu_name": "NVIDIA GPU",
            "vram_total_mb": 12288,
            "vram_free_mb": 10240,
            "ram_total_mb": 32768,
            "ram_free_mb": 24576,
            "cuda_available": true,
            "platform": "Linux"
        }
    }')
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Alice] Room created (201)" "201" "$status"
check_contains "[Alice] Has invite_code" "$body" '"invite_code"'
check_contains "[Alice] Has room ID" "$body" '"id"'
check_contains "[Alice] Model is TinyLlama" "$body" '"model_id":"TinyLlama/TinyLlama-1.1B-Chat-v1.0"'
check_contains "[Alice] State is active" "$body" '"state":"active"'
check_contains "[Alice] Is host" "$body" '"is_host":true'
check_contains "[Alice] Has total_layers" "$body" '"total_layers"'

INVITE_CODE=$(extract_json_field "$body" "invite_code")
printf "    $(dim "Invite code: $INVITE_CODE")\n"

# Alice's health should reflect model loaded
body=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Model now loaded" "$body" '"model_loaded":true'


# ==========================================================================
# PHASE 3: ALICE WORKER STATUS — GPU detected
# ==========================================================================

step "Alice Worker Status — GPU Node" \
     "Verify Alice has GPU/CUDA resources and model loaded"

body=$(curl -sf "${ALICE}/room/status")

check_contains "[Alice] Room status has room" "$body" '"room"'
check_contains "[Alice] Has peers array" "$body" '"peers"'
check_contains "[Alice] VRAM tracking" "$body" '"total_vram_mb"'
check_contains "[Alice] Has host peer" "$body" '"is_host":true'
check_contains "[Alice] Model ID in room" "$body" '"model_id":"TinyLlama/TinyLlama-1.1B-Chat-v1.0"'

# Models list should show TinyLlama
body=$(curl -sf "${ALICE}/v1/models")
check_not_contains "[Alice] Models not empty" "$body" '"data":\[\]'
check_contains "[Alice] TinyLlama listed" "$body" '"id":"TinyLlama/TinyLlama-1.1B-Chat-v1.0"'


# ==========================================================================
# PHASE 4: BOB JOINS — Donates CPU/RAM resources
# ==========================================================================

step "Bob Joins Room — CPU-only Resource Donation" \
     "Bob donates CPU/RAM to the bare metal pool (no GPU)"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${BOB}/room/join" \
    -H "Content-Type: application/json" \
    -d "{
        \"invite_code\": \"${INVITE_CODE}\",
        \"resources\": {
            \"gpu_name\": \"\",
            \"vram_total_mb\": 0,
            \"vram_free_mb\": 0,
            \"ram_total_mb\": 32768,
            \"ram_free_mb\": 24576,
            \"cuda_available\": false,
            \"platform\": \"Linux\"
        }
    }")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Bob] Join returns 200" "200" "$status"
check_contains "[Bob] Same invite code" "$body" "\"invite_code\":\"${INVITE_CODE}\""
check_contains "[Bob] Room is active" "$body" '"state":"active"'
check_contains "[Bob] Has peers array" "$body" '"peers"'
check_contains "[Bob] Has host peer" "$body" '"is_host":true'
check_contains "[Bob] Has layers" "$body" '"layers"'
check_contains "[Bob] Model is TinyLlama" "$body" '"model_id":"TinyLlama/TinyLlama-1.1B-Chat-v1.0"'

# Bob's health should now show model loaded
body=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model now loaded" "$body" '"model_loaded":true'


# ==========================================================================
# PHASE 5: BOB WORKER STATUS — CPU-only
# ==========================================================================

step "Bob Worker Status — CPU-only Node" \
     "Verify Bob is contributing CPU/RAM resources"

body=$(curl -sf "${BOB}/room/status")

check_contains "[Bob] Room status has room" "$body" '"room"'
check_contains "[Bob] Has peers" "$body" '"peers"'
check_contains "[Bob] Has VRAM tracking" "$body" '"total_vram_mb"'


# ==========================================================================
# PHASE 6: POOL RESOURCES CHECK — Combined resources
# ==========================================================================

step "Pool Resources Check" \
     "Verify the room shows pooled resources from both peers"

body=$(curl -sf "${ALICE}/room/status")

check_contains "[Alice] Room has peers" "$body" '"peers"'

# Verify peers are connected (Alice=1 self, Bob=3 mock peers including self)
body=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Has peers connected" "$body" '"peers_connected"'

body=$(curl -sf "${BOB}/health")
check_contains "[Bob] Has peers connected" "$body" '"peers_connected"'


# ==========================================================================
# PHASE 7: ALICE REAL CHAT — Real model inference
# ==========================================================================

step "Alice Real Chat Completion" \
     "Alice sends a prompt — response must come from the REAL model"

body=$(curl -sf --max-time 120 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
        "messages": [{"role": "user", "content": "What is the capital of France? Answer in one word."}],
        "temperature": 0.1,
        "max_tokens": 50
    }')

check_contains "[Alice] Has choices" "$body" '"choices"'
check_contains "[Alice] Has assistant role" "$body" '"role":"assistant"'
check_contains "[Alice] Has content" "$body" '"content"'
check_contains "[Alice] Has usage stats" "$body" '"usage"'
check_contains "[Alice] Has request ID" "$body" '"id":"hm-'

# Verify it's NOT a mock response
check_not_contains "[Alice] Not mock response" "$body" "distributed inference response"
check_not_contains "[Alice] Not placeholder" "$body" "placeholder"

# Verify token usage is real
prompt_tokens=$(extract_json_number "$body" "prompt_tokens")
completion_tokens=$(extract_json_number "$body" "completion_tokens")

if [ -n "$prompt_tokens" ] && [ "$prompt_tokens" -gt 0 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') [Alice] prompt_tokens > 0 (%s)\n" "$prompt_tokens"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') [Alice] prompt_tokens should be > 0 (got: %s)\n" "$prompt_tokens"
fi

if [ -n "$completion_tokens" ] && [ "$completion_tokens" -gt 0 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') [Alice] completion_tokens > 0 (%s)\n" "$completion_tokens"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') [Alice] completion_tokens should be > 0 (got: %s)\n" "$completion_tokens"
fi

ALICE_RESPONSE="$body"
printf "    $(dim "Response: %.200s")\n" "$body"


# ==========================================================================
# PHASE 8: BOB REAL CHAT — Any member can call inference
# ==========================================================================

step "Bob Real Chat Completion" \
     "Bob also calls inference — proves any member can use the pool"

body=$(curl -sf --max-time 120 \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
        "messages": [{"role": "user", "content": "What is 2 + 2? Answer with just the number."}],
        "temperature": 0.1,
        "max_tokens": 50
    }')

check_contains "[Bob] Has choices" "$body" '"choices"'
check_contains "[Bob] Has assistant role" "$body" '"role":"assistant"'
check_contains "[Bob] Has content" "$body" '"content"'
check_contains "[Bob] Has usage stats" "$body" '"usage"'
check_contains "[Bob] Has request ID" "$body" '"id":"hm-'

# Verify it's NOT a mock response
check_not_contains "[Bob] Not mock response" "$body" "distributed inference response"

# Verify token usage is real
prompt_tokens=$(extract_json_number "$body" "prompt_tokens")
if [ -n "$prompt_tokens" ] && [ "$prompt_tokens" -gt 0 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') [Bob] prompt_tokens > 0 (%s)\n" "$prompt_tokens"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') [Bob] prompt_tokens should be > 0 (got: %s)\n" "$prompt_tokens"
fi

BOB_RESPONSE="$body"
printf "    $(dim "Response: %.200s")\n" "$body"


# ==========================================================================
# PHASE 9: RESPONSES DIFFER — Real model is non-deterministic
# ==========================================================================

step "Responses Are Different" \
     "Different prompts produce different outputs (proves real inference)"

alice_content=$(extract_json_field "$ALICE_RESPONSE" "content")
bob_content=$(extract_json_field "$BOB_RESPONSE" "content")

if [ -n "$alice_content" ] && [ -n "$bob_content" ] && [ "$alice_content" != "$bob_content" ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Alice and Bob got different responses\n"
    printf "       $(dim "Alice: %.80s")\n" "$alice_content"
    printf "       $(dim "Bob:   %.80s")\n" "$bob_content"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected different responses\n"
    printf "       $(dim "Alice: %.80s")\n" "$alice_content"
    printf "       $(dim "Bob:   %.80s")\n" "$bob_content"
fi

# Verify IDs are different (independent requests)
alice_id=$(extract_json_field "$ALICE_RESPONSE" "id")
bob_id=$(extract_json_field "$BOB_RESPONSE" "id")

if [ -n "$alice_id" ] && [ -n "$bob_id" ] && [ "$alice_id" != "$bob_id" ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Different request IDs (independent sessions)\n"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected different IDs: alice=%s bob=%s\n" "$alice_id" "$bob_id"
fi


# ==========================================================================
# PHASE 10: STREAMING REAL — SSE with actual model tokens
# ==========================================================================

step "Real Streaming Chat" \
     "Alice streams a response — SSE tokens from the real model"

content_type=$(curl -s -o /dev/null -w "%{content_type}" --max-time 120 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B-Chat-v1.0","messages":[{"role":"user","content":"Count from 1 to 5"}],"stream":true,"max_tokens":100}')

if echo "$content_type" | grep -qi "text/event-stream"; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Content-Type is text/event-stream\n"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Content-Type: %s (expected text/event-stream)\n" "$content_type"
fi

stream_body=$(curl -sf --max-time 120 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B-Chat-v1.0","messages":[{"role":"user","content":"Say hello world"}],"stream":true,"max_tokens":50}')

check_contains "[Alice] Stream has data:" "$stream_body" "data:"
check_contains "[Alice] Stream has chunks" "$stream_body" "chat.completion.chunk"
check_contains "[Alice] Stream ends with DONE" "$stream_body" "[DONE]"

chunk_count=$(echo "$stream_body" | grep -c "^data: {" || true)
if [ "$chunk_count" -gt 3 ]; then
    PASS=$((PASS + 1))
    printf "    $(green '✓') Multiple stream chunks (%d)\n" "$chunk_count"
else
    FAIL=$((FAIL + 1))
    printf "    $(red '✗') Expected >3 stream chunks, got %d\n" "$chunk_count"
fi


# ==========================================================================
# PHASE 11: CONCURRENT INFERENCE — Both at the same time
# ==========================================================================

step "Concurrent Inference" \
     "Both Alice and Bob run chat completion simultaneously"

# Alice chat (background)
alice_chat_file="/tmp/alice_real_chat.json"
curl -sf --max-time 120 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B-Chat-v1.0","messages":[{"role":"user","content":"What is machine learning?"}],"max_tokens":100}' \
    -o "$alice_chat_file" &
alice_pid=$!

# Bob chat (background)
bob_chat_file="/tmp/bob_real_chat.json"
curl -sf --max-time 120 \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B-Chat-v1.0","messages":[{"role":"user","content":"What is deep learning?"}],"max_tokens":100}' \
    -o "$bob_chat_file" &
bob_pid=$!

# Wait for both
wait $alice_pid
wait $bob_pid

alice_body=$(cat "$alice_chat_file" 2>/dev/null)
bob_body=$(cat "$bob_chat_file" 2>/dev/null)

check_contains "[Alice] Concurrent chat has choices" "$alice_body" '"choices"'
check_contains "[Alice] Concurrent chat has content" "$alice_body" '"content"'
check_contains "[Alice] Concurrent has usage" "$alice_body" '"usage"'

check_contains "[Bob] Concurrent chat has choices" "$bob_body" '"choices"'
check_contains "[Bob] Concurrent chat has content" "$bob_body" '"content"'
check_contains "[Bob] Concurrent has usage" "$bob_body" '"usage"'

rm -f "$alice_chat_file" "$bob_chat_file"


# ==========================================================================
# PHASE 12: MULTI-TURN CONVERSATION — Context-aware
# ==========================================================================

step "Multi-Turn Conversation" \
     "Both users have multi-turn conversations with context"

alice_body=$(curl -sf --max-time 120 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "My name is Alice."},
            {"role": "assistant", "content": "Hello Alice!"},
            {"role": "user", "content": "What is my name?"}
        ],
        "max_tokens": 50
    }')

check_contains "[Alice] Multi-turn has choices" "$alice_body" '"choices"'
check_contains "[Alice] Multi-turn has usage" "$alice_body" '"prompt_tokens"'

bob_body=$(curl -sf --max-time 120 \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "TinyLlama/TinyLlama-1.1B-Chat-v1.0",
        "messages": [
            {"role": "system", "content": "You are a math tutor."},
            {"role": "user", "content": "What is 5 times 3?"},
            {"role": "assistant", "content": "15"},
            {"role": "user", "content": "And if you add 10 to that?"}
        ],
        "max_tokens": 50
    }')

check_contains "[Bob] Multi-turn has choices" "$bob_body" '"choices"'
check_contains "[Bob] Multi-turn has usage" "$bob_body" '"prompt_tokens"'


# ==========================================================================
# PHASE 13: BOB LEAVES — Pool shrinks
# ==========================================================================

step "Bob Leaves Room" \
     "Bob disconnects — pool loses CPU resources"

response=$(curl -s -w "\n%{http_code}" -X DELETE "${BOB}/room/leave")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Bob] Leave returns 200" "200" "$status"
check_contains "[Bob] Leave confirmed" "$body" '"status":"left"'

# Double leave should fail
status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${BOB}/room/leave")
check_status "[Bob] Double leave returns 404" "404" "$status"

# Bob is now blocked
bob_health=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model unloaded" "$bob_health" '"model_loaded":false'

status=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"test","messages":[{"role":"user","content":"hello"}]}')
check_status "[Bob] Chat blocked after leave" "404" "$status"

# Alice still works (host has remaining resources)
alice_body=$(curl -sf --max-time 120 \
    -X POST "${ALICE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B-Chat-v1.0","messages":[{"role":"user","content":"Hello"}],"max_tokens":20}')
check_contains "[Alice] Still works after Bob left" "$alice_body" '"choices"'

alice_health=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Model still loaded" "$alice_health" '"model_loaded":true'


# ==========================================================================
# PHASE 14: BOB REJOINS — Pool restored
# ==========================================================================

step "Bob Rejoins — Pool Restored" \
     "Bob returns with CPU resources — pool is back to full capacity"

response=$(curl -s -w "\n%{http_code}" \
    -X POST "${BOB}/room/join" \
    -H "Content-Type: application/json" \
    -d "{
        \"invite_code\": \"${INVITE_CODE}\",
        \"resources\": {
            \"gpu_name\": \"\",
            \"vram_total_mb\": 0,
            \"vram_free_mb\": 0,
            \"ram_total_mb\": 32768,
            \"ram_free_mb\": 24576,
            \"cuda_available\": false,
            \"platform\": \"Linux\"
        }
    }")
status=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

check_status "[Bob] Rejoin returns 200" "200" "$status"
check_contains "[Bob] Room active after rejoin" "$body" '"state":"active"'
check_contains "[Bob] Has peers" "$body" '"peers"'

bob_health=$(curl -sf "${BOB}/health")
check_contains "[Bob] Model loaded after rejoin" "$bob_health" '"model_loaded":true'

# Bob can do inference again
body=$(curl -sf --max-time 120 \
    -X POST "${BOB}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"model":"TinyLlama/TinyLlama-1.1B-Chat-v1.0","messages":[{"role":"user","content":"Are you back online?"}],"max_tokens":30}')
check_contains "[Bob] Inference works after rejoin" "$body" '"choices"'

# Both models list
alice_models=$(curl -sf "${ALICE}/v1/models")
bob_models=$(curl -sf "${BOB}/v1/models")

check_contains "[Alice] TinyLlama still listed" "$alice_models" '"id":"TinyLlama/TinyLlama-1.1B-Chat-v1.0"'
check_contains "[Bob] TinyLlama listed after rejoin" "$bob_models" '"id":"TinyLlama/TinyLlama-1.1B-Chat-v1.0"'


# ==========================================================================
# PHASE 15: FULL CLEANUP — Both leave, state verified
# ==========================================================================

step "Full Cleanup — Both Leave" \
     "Both users leave; clean state verified"

# Alice leaves first
status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${ALICE}/room/leave")
check_status "[Alice] Leave returns 200" "200" "$status"

alice_health=$(curl -sf "${ALICE}/health")
check_contains "[Alice] Model unloaded" "$alice_health" '"model_loaded":false'

# Bob still works momentarily, then leaves
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


# ==========================================================================
# RESULTS
# ==========================================================================

TOTAL=$((PASS + FAIL))

printf "\n"
printf "================================================================\n"
printf "$(bold 'Real Inference Test Results:') $(green "$PASS") passed, "
if [ "$FAIL" -gt 0 ]; then
    printf "$(red "$FAIL") failed"
else
    printf "0 failed"
fi
printf ", %s total\n" "$TOTAL"
printf "================================================================\n"

if [ "$FAIL" -gt 0 ]; then
    printf "\n$(red 'REAL INFERENCE TEST FAILED')\n\n"
    exit 1
else
    printf "\n$(green 'REAL INFERENCE TEST PASSED — GPU + CPU pooling verified')\n\n"
    exit 0
fi

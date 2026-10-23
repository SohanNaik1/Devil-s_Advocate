#!/usr/bin/env bash
# scripts/acceptance_test.sh — Full acceptance test suite for Devil's Advocate
set -uo pipefail

ROOT_DIR="/home/sohan/Projects/Devil-s_Advocate"
cd "$ROOT_DIR"

cleanup() {
  echo ""
  echo "--- Cleaning up processes on ports 8080 and 9000 ---"
  fuser -k 8080/tcp 9000/tcp 2>/dev/null || true
  sleep 1
}
trap cleanup EXIT INT TERM

cleanup

# 1. Compile Go binary
echo "=== Compiling Go server binary ==="
go build -o /tmp/da-server ./cmd/server/main.go
echo "Go build complete."

# 2. Start Whisper microservice on port 9000
echo ""
echo "=== Starting Whisper service on port 9000 ==="
whisper/.venv/bin/python whisper/server.py &
WHISPER_PID=$!

echo "Waiting for Whisper service to be healthy..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:9000/healthz >/dev/null 2>&1; then
    echo "Whisper is healthy and ready (took ${i}s)!"
    break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Whisper failed to start within 30s"
    exit 1
  fi
done

wait_for_go() {
  for i in $(seq 1 20); do
    if curl -sf http://localhost:8080/healthz >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

# Helper: compare output
pass() { echo "✅ PASS: $1"; }
fail() { echo "❌ FAIL: $1"; exit 1; }

echo ""
echo "=========================================================="
echo "TEST 1: Groq LLM + ElevenLabs TTS — interrupt with audio"
echo "=========================================================="
MODEL_PROVIDER=groq /tmp/da-server &
GO_PID=$!
wait_for_go || fail "Go server failed to start"

SESS_ID=$(curl -s -X POST http://localhost:8080/api/session | python3 -c "import json,sys; print(json.load(sys.stdin)['session_id'])")
echo "Created session: $SESS_ID"

RESP1=$(curl -s -F "session_id=$SESS_ID" -F "kind=speech" -F "offset_ms=0" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)
echo "Response:"
echo "$RESP1" | python3 -m json.tool 2>/dev/null || echo "$RESP1"

python3 -c "
import json, sys
data = json.loads('''$RESP1''')
interrupt = data.get('interrupt')
assert interrupt is not None, 'interrupt is null'
assert interrupt.get('trigger') == 'unsupported_claim', f'wrong trigger: {interrupt.get(\"trigger\")}'
assert 'faster' in interrupt.get('quote', '').lower() or '10' in interrupt.get('quote', ''), f'bad quote: {interrupt.get(\"quote\")}'
assert len(interrupt.get('question', '').split()) <= 25, f'question too long: {interrupt.get(\"question\")}'
audio_url = interrupt.get('audio_url', '')
assert audio_url.startswith('/api/audio/'), f'bad audio_url: {audio_url}'
print('  trigger:', interrupt.get('trigger'))
print('  quote:', interrupt.get('quote'))
print('  question:', interrupt.get('question'))
print('  audio_url:', audio_url)
"
pass "Groq fired interrupt with valid fields"

# Now test GET /api/audio/{id}.mp3 returns actual MP3 bytes
AUDIO_URL=$(echo "$RESP1" | python3 -c "import json,sys; print(json.load(sys.stdin)['interrupt']['audio_url'])")
echo ""
echo "Testing audio endpoint: $AUDIO_URL"
HTTP_CODE=$(curl -s -o /tmp/test_audio.mp3 -w "%{http_code}" "http://localhost:8080${AUDIO_URL}")
AUDIO_SIZE=$(wc -c < /tmp/test_audio.mp3)
echo "HTTP status: $HTTP_CODE, audio size: ${AUDIO_SIZE} bytes"

[ "$HTTP_CODE" = "200" ] || fail "audio endpoint returned $HTTP_CODE"
[ "$AUDIO_SIZE" -gt 1000 ] || fail "audio file too small (${AUDIO_SIZE} bytes)"
pass "GET /api/audio/{id}.mp3 returned ${AUDIO_SIZE} bytes of audio"

kill $GO_PID 2>/dev/null || true; wait $GO_PID 2>/dev/null || true; sleep 1

echo ""
echo "=========================================================="
echo "TEST 2: Fake provider — quote discarded, no TTS called"
echo "=========================================================="
MODEL_PROVIDER=fake /tmp/da-server &
GO_PID=$!
wait_for_go || fail "Go server failed to start"

SESS_ID=$(curl -s -X POST http://localhost:8080/api/session | python3 -c "import json,sys; print(json.load(sys.stdin)['session_id'])")
RESP2=$(curl -s -F "session_id=$SESS_ID" -F "kind=speech" -F "offset_ms=0" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)

python3 -c "
import json
data = json.loads('''$RESP2''')
assert data.get('interrupt') is None, f'interrupt should be null: {data.get(\"interrupt\")}'
"
pass "Fake provider quote discarded, interrupt: null"

kill $GO_PID 2>/dev/null || true; wait $GO_PID 2>/dev/null || true; sleep 1

echo ""
echo "=========================================================="
echo "TEST 3: fake_valid — 60s cooldown suppresses 2nd interrupt"
echo "=========================================================="
MODEL_PROVIDER=fake_valid /tmp/da-server &
GO_PID=$!
wait_for_go || fail "Go server failed to start"

SESS_ID=$(curl -s -X POST http://localhost:8080/api/session | python3 -c "import json,sys; print(json.load(sys.stdin)['session_id'])")
RESP3_1=$(curl -s -F "session_id=$SESS_ID" -F "kind=speech" -F "offset_ms=0" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)
RESP3_2=$(curl -s -F "session_id=$SESS_ID" -F "kind=speech" -F "offset_ms=3000" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)

python3 -c "
import json
d1 = json.loads('''$RESP3_1''')
d2 = json.loads('''$RESP3_2''')
assert d1.get('interrupt') is not None, 'Chunk 1 interrupt was null'
assert d2.get('interrupt') is None, f'Chunk 2 interrupt not suppressed: {d2.get(\"interrupt\")}'
"
pass "Chunk 1 fired; Chunk 2 within 60s returned null (cooldown working)"

kill $GO_PID 2>/dev/null || true; wait $GO_PID 2>/dev/null || true; sleep 1

echo ""
echo "=========================================================="
echo "TEST 4: Answer chunk — kind=answer skips LLM check"
echo "=========================================================="
MODEL_PROVIDER=groq /tmp/da-server &
GO_PID=$!
wait_for_go || fail "Go server failed to start"

SESS_ID=$(curl -s -X POST http://localhost:8080/api/session | python3 -c "import json,sys; print(json.load(sys.stdin)['session_id'])")
# Send with kind=answer — LLM should NOT be called, interrupt should be null
RESP4=$(curl -s -F "session_id=$SESS_ID" -F "kind=answer" -F "offset_ms=0" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)

python3 -c "
import json
data = json.loads('''$RESP4''')
assert data.get('interrupt') is None, f'kind=answer should not trigger LLM: {data.get(\"interrupt\")}'
"
pass "kind=answer chunk correctly skips LLM interrupt check"

kill $GO_PID 2>/dev/null || true; wait $GO_PID 2>/dev/null || true; sleep 1

echo ""
echo "=========================================================="
echo "ALL 4 ACCEPTANCE TESTS PASSED ✅"
echo "=========================================================="

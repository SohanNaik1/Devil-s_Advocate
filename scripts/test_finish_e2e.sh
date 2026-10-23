#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="/home/sohan/Projects/Devil-s_Advocate"
cd "$ROOT_DIR"

cleanup() {
  fuser -k 8080/tcp 9000/tcp 2>/dev/null || true
  sleep 1
}
trap cleanup EXIT INT TERM
cleanup

echo "=== Compiling server binary ==="
go build -o /tmp/da-server ./cmd/server/main.go

echo "=== Starting Whisper service ==="
whisper/.venv/bin/python whisper/server.py &
WHISPER_PID=$!

for i in $(seq 1 30); do
  if curl -sf http://localhost:9000/healthz >/dev/null 2>&1; then
    echo "Whisper is ready!"
    break
  fi
  sleep 1
done

echo "=== Starting Go Server with Groq ==="
MODEL_PROVIDER=groq /tmp/da-server &
GO_PID=$!

for i in $(seq 1 20); do
  if curl -sf http://localhost:8080/healthz >/dev/null 2>&1; then
    echo "Go server is ready!"
    break
  fi
  sleep 0.5
done

echo "1. Creating session..."
SESS_ID=$(curl -s -X POST http://localhost:8080/api/session | python3 -c "import json,sys; print(json.load(sys.stdin)['session_id'])")
echo "Session ID: $SESS_ID"

echo "2. Sending speech chunk with bold claim..."
CHUNK_RESP=$(curl -s -F "session_id=$SESS_ID" -F "kind=speech" -F "offset_ms=0" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)
echo "Chunk response received."

echo "3. Sending answer chunk..."
ANS_RESP=$(curl -s -F "session_id=$SESS_ID" -F "kind=answer" -F "offset_ms=3500" -F "audio=@testdata/claim.mp3" http://localhost:8080/api/chunk)
echo "Answer response received."

echo "4. Finalizing presentation via POST /api/finish..."
FINISH_RESP=$(curl -s -H "Content-Type: application/json" -d "{\"session_id\":\"$SESS_ID\", \"duration_seconds\": 145}" http://localhost:8080/api/finish)

python3 -c "
import json
data = json.loads('''$FINISH_RESP''')
assert data['session_id'] == '$SESS_ID'
assert 'overall' in data, 'missing overall'
assert 'delivery' in data, 'missing delivery'
assert 'content' in data, 'missing content'
assert 'interruptions' in data, 'missing interruptions'
assert 'went_well' in data, 'missing went_well'
assert 'to_fix' in data, 'missing to_fix'
print('Scorecard Summary:')
print(f'  Overall: {data[\"overall\"]}/100')
print(f'  Delivery: {data[\"delivery\"][\"score\"] if isinstance(data[\"delivery\"], dict) else data[\"delivery\"]}/100')
print(f'  Content: {data[\"content\"][\"score\"] if isinstance(data[\"content\"], dict) else data[\"content\"]}/100')
print(f'  Went Well count: {len(data.get(\"went_well\", []))}')
print(f'  To Fix count: {len(data.get(\"to_fix\", []))}')
print(f'  Interruptions count: {len(data.get(\"interruptions\", []))}')
"

echo "5. Verifying GET /api/report/$SESS_ID..."
SAVED_REPORT=$(curl -s "http://localhost:8080/api/report/$SESS_ID")
python3 -c "
import json
data = json.loads('''$SAVED_REPORT''')
assert data['session_id'] == '$SESS_ID'
print('  GET /api/report verified successfully!')
"

echo "6. Verifying GET /history..."
HIST=$(curl -s http://localhost:8080/history)
python3 -c "
import json
data = json.loads('''$HIST''')
assert isinstance(data, list) and len(data) > 0
assert data[0]['session_id'] == '$SESS_ID'
print(f'  GET /history returned {len(data)} sessions, latest is {data[0][\"session_id\"]}')
"

echo "7. Verifying static frontend index.html served at /..."
FRONTEND_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/)
echo "  Frontend HTTP Code: $FRONTEND_CODE"
[ "$FRONTEND_CODE" = "200" ]

echo ""
echo "=========================================================="
echo "ALL END-TO-END PRESENTATION & REPORTING TESTS PASSED! ✅"
echo "=========================================================="

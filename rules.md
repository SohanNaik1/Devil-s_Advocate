# Project: Devil's Advocate
A live presentation rehearsal tool. The user presents out loud, an AI panelist interrupts if it has something specific to challenge (quoting exact words), and the user receives a scorecard at the end.

## Tech Stack
- **Backend:** Go (standard library `net/http`). No heavy web frameworks.
- **Speech-to-Text:** Python service using `faster-whisper` (model `small`, int8, CPU).
- **Frontend:** Vanilla JS and HTML (`index.html`, `app.js`). Uses `MediaRecorder`.
- **Reasoning Model:** Gemma via provider switch (aistudio, gradient, ollama). Must enforce strict JSON.
- **Voice Out:** ElevenLabs (lowest latency).
- **Cloud / Infrastructure:** AWS (S3 for audio/reports, DynamoDB for session summaries, API Gateway + Lambda for `/history`). 

## Core Rules for Antigravity
1. **No Scope Creep:** Only build exactly what is asked in the current prompt.
2. **Exact Paths & Names:** Use the exact folder structure, endpoint URLs, and JSON keys listed below.
3. **No DigitalOcean:** We are running locally/custom server. Do not write DO-specific deployment code.
4. **Resilience:** LLM calls must validate JSON and retry exactly once on failure. Interruption quotes must be verified word-for-word against the transcript.

## Repository Layout
devils-advocate/
├── cmd/server/main.go
├── internal/api/          # HTTP handlers
├── internal/asr/          # client for the Whisper service
├── internal/llm/          # provider switch, JSON prompts, quote verification
├── internal/voice/        # ElevenLabs client
├── internal/metrics/      # pace, fillers, pauses
├── internal/report/       # final report builder
├── internal/store/        # S3 and DynamoDB
├── web/index.html
├── web/app.js
├── whisper/server.py
├── lambda/history/        # Lambda for GET /history
├── .env.example
├── LICENSE
└── README.md

## API Contracts

### Backend Endpoints (Go)
- `POST /api/session` -> `{"session_id": "..."}`
- `POST /api/chunk` (multipart: `session_id`, `kind` [speech/answer], `offset_ms`, audio file) -> `{"text": "...", "words": [{"word": "...", "start_ms": 0, "end_ms": 0}], "interrupt": null | {...}}`
- `GET /api/audio/{id}.mp3` -> returns audio file
- `POST /api/finish` `{"session_id": "..."}` -> returns Report JSON
- `GET /api/report/{session_id}` -> returns saved Report JSON

### Whisper Endpoint (Python)
- `POST localhost:9000/transcribe` (multipart audio) -> `{"text": "...", "duration_ms": 0, "words": [{"word": "...", "start_ms": 0, "end_ms": 0}]}`

### Lambda Endpoint
- `GET /history` -> returns array of last 10 sessions (session_id, created_at, overall, delivery, content)

## JSON Shapes

**Interrupt Decision (from Model):**
`{"interrupt": false}` 
OR 
`{"interrupt": true, "trigger": "unsupported_claim | contradiction | jargon | vague", "quote": "exact words from transcript", "question": "under 25 words"}`

**Interrupt Object (Returned to Frontend):**
`{"id": "abc123", "trigger": "...", "quote": "...", "question": "...", "audio_url": "/api/audio/abc123.mp3"}`

**Report JSON:**
{
  "session_id": "...",
  "duration_s": 0,
  "overall": 0,
  "delivery": {
    "score": 0, "wpm": 0, "wpm_by_30s": [], "filler_count": 0,
    "fillers": {}, "fillers_per_min": 0, "long_pauses": [{"start_ms": 0, "duration_ms": 0}],
    "longest_pause_ms": 0, "talk_ratio": 0
  },
  "content": {
    "score": 0, "structure": 0, "clarity": 0, "evidence": 0, "answers": 0,
    "went_well": [{"point": "", "quote": ""}],
    "to_fix": [{"point": "", "quote": "", "suggestion": ""}]
  },
  "interruptions": [
    {"at_ms": 0, "trigger": "", "quote": "", "question": "", "answer_text": "", "answer_rating": 0, "response_time_ms": 0}
  ]
}

## Environment Variables (.env)
MODEL_PROVIDER=aistudio
GOOGLE_AI_STUDIO_API_KEY=
GEMMA_MODEL_ID=
ELEVENLABS_API_KEY=
ELEVENLABS_VOICE_ID_HOST=
ELEVENLABS_VOICE_ID_PANELIST=
AWS_REGION=ap-south-1
AWS_S3_BUCKET=
AWS_DYNAMODB_TABLE=sessions


You are Antigravity, an expert coding agent building "Devil's Advocate", a live presentation rehearsal tool. 
Follow these strict rules for every task:
1. Stack: Go (standard library net/http), Python (faster-whisper), Vanilla JS/HTML (no frameworks).
2. Endpoints & JSON: Use the exact paths, endpoint names, and JSON shapes provided in the prompts. Do not alter them.
3. Scope: Write complete, working code for the current task only. Never touch files or features listed as "Out of scope".
4. Environment: We are running locally/custom server. Do not write deployment code for DigitalOcean.
5. Output: Provide the code directly. Do not explain the code unless asked.
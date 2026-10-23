# Devil's Advocate

> AI-powered debate practice tool — pits you against an LLM-backed panel that argues the opposite side of any proposition.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## Stack

| Layer | Technology |
|---|---|
| API server | Go (`net/http`) |
| LLM backend | Google AI Studio (Gemini / Gemma) — switchable |
| ASR | OpenAI Whisper (self-hosted Python sidecar) |
| TTS | ElevenLabs |
| Storage | AWS S3 (audio) · DynamoDB (sessions) |
| Frontend | Vanilla HTML / JS |

---

## Quick Start

### 1. Copy env template

```bash
cp .env.example .env
# Fill in your API keys
```

### 2. Run the server

```bash
go run ./cmd/server/main.go
```

### 3. Verify health

```bash
curl localhost:8080/healthz
# → ok
```

---

## Project Layout

```
.
├── cmd/
│   └── server/          # Binary entry point
│       └── main.go
├── internal/
│   ├── api/             # HTTP routing & handlers
│   ├── asr/             # Whisper ASR client
│   ├── llm/             # LLM provider switch (aistudio | gemma-local)
│   ├── voice/           # ElevenLabs TTS client
│   ├── metrics/         # Runtime metrics
│   ├── report/          # Session report generation
│   └── store/           # DynamoDB / S3 persistence
├── lambda/
│   └── history/         # AWS Lambda: session history
├── web/
│   ├── index.html       # Frontend entry point
│   └── app.js           # Frontend logic
├── whisper/
│   └── server.py        # Whisper ASR sidecar
├── .env.example         # Environment variable template
└── LICENSE
```

---

## Environment Variables

| Variable | Description |
|---|---|
| `MODEL_PROVIDER` | `aistudio` or `gemma-local` |
| `GOOGLE_AI_STUDIO_API_KEY` | API key for Google AI Studio |
| `GEMMA_MODEL_ID` | Model ID for Gemma (when using aistudio) |
| `ELEVENLABS_API_KEY` | ElevenLabs TTS API key |
| `ELEVENLABS_VOICE_ID_HOST` | Voice ID for the AI host persona |
| `ELEVENLABS_VOICE_ID_PANELIST` | Voice ID for the AI panelist persona |
| `AWS_REGION` | AWS region (default: `ap-south-1`) |
| `AWS_S3_BUCKET` | S3 bucket for audio storage |
| `AWS_DYNAMODB_TABLE` | DynamoDB table for sessions |

---

## License

[MIT](LICENSE) © 2026 Sohan Naik

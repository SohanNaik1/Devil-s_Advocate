# Devil's Advocate

A live presentation stress-testing platform that challenges speakers in real time with an adversarial AI panelist and produces comprehensive speaking delivery scorecards.

---

## System Overview

Devil's Advocate provides an interactive rehearsal environment for high-stakes pitches, technical defenses, and keynote presentations. As the user speaks, the system continuously analyzes transcription chunks for unsubstantiated assertions, logical contradictions, or unexplained jargon. When a trigger condition is met, the AI panelist verbally interrupts the speaker through synthesized speech, citing the speaker's exact words and demanding clarification.

```
+-----------------------------------------------------------------------------------+
|                                  BROWSER CLIENT                                   |
|   MediaRecorder + AudioContext (1.2s silence detection) <-> Live Scorecard UI    |
+-----------------------------------------------------------------------------------+
                                         |
                                         | HTTP / Multipart Audio Chunks
                                         v
+-----------------------------------------------------------------------------------+
|                                GO BACKEND SERVER                                  |
|   HTTP Gateway (:8080) | In-Memory Session Store | Strict Quote Verification Loop|
+-----------------------------------------------------------------------------------+
         |                                                 |
         | HTTP                                            | HTTP (JSON Completion)
         v                                                 v
+-----------------------------------+     +-----------------------------------------+
|     WHISPER PYTHON SERVICE        |     |          GROQ REASONING ENGINE          |
| faster-whisper (:9000, CPU/int8)  |     | Gemma / LLaMA (Strict JSON Validation)   |
+-----------------------------------+     +-----------------------------------------+
                                                           |
                                                           | Text-to-Speech (MP3)
                                                           v
                                          +-----------------------------------------+
                                          |          ELEVENLABS TTS API             |
                                          | eleven_turbo_v2_5 (Low-latency audio)   |
                                          +-----------------------------------------+
```

---

## Core Capabilities

- **Zero-Latency Audio Segmentation:** The client utilizes Web Audio API `AnalyserNode` monitoring to automatically detect natural pauses (1.2 seconds of silence) and flush audio chunks without manual intervention.
- **Verbatim Quote Verification:** All interruption candidates returned by the reasoning engine are checked word-for-word against the active transcript in Go. Hallucinated or misattributed quotes are discarded before synthesis.
- **Echo Prevention & State Management:** When an interruption fires, microphone input is muted while synthesized audio plays. Recording resumes in answer mode upon playback completion.
- **Algorithmic Delivery Analytics:** Computes words per minute (WPM), tracks filler word taxonomy ("um", "like", "basically", "you know"), detects silence pauses exceeding 2.0 seconds, and evaluates overall composure.

---

## Technology Stack

| Layer | Component | Details |
|---|---|---|
| **Backend Gateway** | Go (`net/http`) | Standard library HTTP server with in-memory session and report store |
| **Speech-to-Text (ASR)** | Python (`faster-whisper`) | Model `small`, int8 quantization, CPU execution (MIT License) |
| **Reasoning Engine** | Gemma via Groq | Sub-second inference, strict JSON schema output, automatic retry |
| **Speech Synthesis (TTS)**| ElevenLabs | Model `eleven_turbo_v2_5`, in-memory audio caching and streaming |
| **Frontend Client** | HTML5 / CSS3 / JavaScript | Vanilla implementation using `MediaRecorder` and `AudioContext` |
| **Deployment Model** | Local Microservices | Self-contained local architecture with zero external cloud lock-in |

---

## Repository Structure

```
.
|-- cmd/
|   `-- server/
|       `-- main.go              # Go server entry point & HTTP router
|-- internal/
|   |-- api/                     # HTTP handlers (session, chunk, finish, history)
|   |-- asr/                     # HTTP client for Whisper service
|   |-- llm/                     # LLM provider switch & quote verification
|   |-- metrics/                 # WPM, filler word, and pause calculations
|   |-- report/                  # Final scorecard and qualitative feedback builder
|   |-- store/                   # Thread-safe in-memory session registry
|   `-- voice/                   # ElevenLabs TTS client and audio cache
|-- static/
|   `-- audio/                   # Static challenge audio assets
|-- web/
|   |-- index.html               # Main presentation and scorecard interface
|   `-- app.js                   # Client-side audio recording and event loops
|-- whisper/
|   |-- server.py                # Flask transcription microservice
|   `-- requirements.txt         # Python dependencies
|-- scripts/
|   |-- acceptance_test.sh       # Verification test suite
|   `-- test_finish_e2e.sh       # End-to-end pipeline test
|-- .env.example                 # Environment variable template
|-- rules.md                     # API specification and contract definitions
`-- README.md                    # Project documentation
```

---

## Getting Started

### 1. Environment Configuration

Create a `.env` file in the project root:

```bash
MODEL_PROVIDER=groq
GROQ_API_KEY=your_groq_api_key_here
GROQ_MODEL_ID=qwen/qwen3.8-27b
ELEVENLABS_API_KEY=your_elevenlabs_api_key_here
ELEVENLABS_VOICE_ID_PANELIST=pNInz6obpgDQGcFmaJgB
```

### 2. Start the Whisper ASR Service

The transcription service runs as a lightweight Python microservice on port 9000:

```bash
# Using the pre-installed virtual environment
whisper/.venv/bin/python whisper/server.py
```

Verify the service is online:

```bash
curl http://localhost:9000/healthz
# Expected output: ok
```

### 3. Start the Go Application Server

In a separate terminal, launch the Go backend on port 8080:

```bash
go run ./cmd/server/main.go
```

Verify server health:

```bash
curl http://localhost:8080/healthz
# Expected output: ok
```

### 4. Launch Rehearsal

Open your browser and navigate to:

```text
http://localhost:8080
```

---

## Rehearsal Workflow

1. **Start Session:** Click **Start Mic** to initiate the Web Audio recorder and establish a session ID.
2. **Present:** Deliver your presentation into the microphone. Chunks are automatically dispatched when natural pauses occur.
3. **Handle Challenges:** When the panelist detects a questionable claim, synthesized audio plays through your speakers. The microphone automatically resumes once the challenge concludes.
4. **Finalize & Review:** Click **Stop Presentation** at any point to stop recording and display the performance scorecard.

---

## API Contract Reference

### Backend Endpoints (Go, Port 8080)

- `POST /api/session`: Creates a new rehearsal session. Returns `{"session_id": "uuid"}`.
- `POST /api/chunk`: Multipart audio upload (`audio`, `session_id`, `kind`, `offset_ms`). Returns transcribed text, word timestamps, and interruption payload (if triggered).
- `GET /api/audio/{id}`: Streams cached or synthesized MP3 audio for an interruption challenge.
- `POST /api/finish`: Concludes a session and generates the full scorecard report JSON.
- `GET /api/report/{session_id}`: Retrieves the scorecard report for a specific session.
- `GET /history`: Returns a list of past rehearsal session summaries for the sidebar drawer.

### Transcription Endpoint (Python, Port 9000)

- `POST /transcribe`: Accepts binary audio and returns normalized text, duration, and word-level timestamp offsets.

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

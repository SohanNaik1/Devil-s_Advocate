# Devil's Advocate

> A live presentation rehearsal tool that challenges speakers in real-time with an adversarial AI panelist and delivers comprehensive speaking performance scorecards.

---

## ⚡ Tech Stack

- **Backend API:** Go (Standard library `net/http` — high-performance, zero heavy frameworks)
- **Speech-to-Text (ASR):** Python microservice with `faster-whisper` (`small` model, int8 quantization, CPU-optimized — MIT License)
- **Adversarial Reasoning:** Gemma via Groq (ultra low-latency inference, strict JSON schema output with word-for-word quote verification)
- **Voice Synthesis:** ElevenLabs (low-latency adversarial panelist speech synthesis via `eleven_turbo_v2_5`)
- **Frontend:** Vanilla HTML5, modern CSS3, and JavaScript (Web Audio API: `MediaRecorder`, `AudioContext`, and `AnalyserNode` for 1.2s silence detection)
- **Architecture:** 100% local microservice pipeline — No AWS or heavy cloud dependencies required.

---

## 🚀 How to Run Locally

### 1. Configure Environment Variables

Create a `.env` file in the root directory:

```bash
MODEL_PROVIDER=groq
GROQ_API_KEY=your_groq_api_key_here
GROQ_MODEL_ID=qwen/qwen3.8-27b
ELEVENLABS_API_KEY=your_elevenlabs_api_key_here
ELEVENLABS_VOICE_ID_PANELIST=pNInz6obpgDQGcFmaJgB
```

### 2. Start the Whisper Python Service (Port 9000)

```bash
# Using the pre-configured virtual environment:
whisper/.venv/bin/python whisper/server.py
```

*Whisper will load the model and start listening on `http://localhost:9000`.*

### 3. Start the Go Backend Server (Port 8080)

In a new terminal window:

```bash
go run ./cmd/server/main.go
```

*The API server will start on `http://localhost:8080` and statically serve the frontend web interface.*

### 4. Open the Web Application

Navigate to:

```text
http://localhost:8080
```

---

## 🎙️ Rehearsal Flow

1. Click **Start Mic** and present your pitch out loud.
2. The browser automatically records audio and cuts chunks during natural 1.2-second pauses.
3. If an unsupported claim, contradiction, jargon, or vague claim is detected, the **AI panelist interrupts verbally** through your speakers, quoting your exact words.
4. Answer the challenge out loud; recording automatically resumes in answer mode.
5. Click **Stop Presentation** at any time to instantly view your analytical scorecard covering delivery pace (WPM), filler word counts, hesitation pauses, strengths, and critique.

---

## 📄 License

MIT License © 2026 Devil's Advocate Team

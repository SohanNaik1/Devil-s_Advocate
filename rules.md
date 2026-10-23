# Project: Devil's Advocate
A live presentation rehearsal tool. The user presents out loud, an AI panelist interrupts if it has something specific to challenge (quoting exact words), and the user receives a scorecard at the end.

## Tech Stack
- **Backend:** Python (`app.py` / Flask) & Go standard library `net/http`.
- **Speech-to-Text:** Python service using `faster-whisper` (model `small`, int8, CPU).
- **Frontend:** Vanilla JS, HTML, and CSS (`index.html`). Uses `MediaRecorder` and `AudioContext`.
- **Reasoning Model:** Gemma / Groq / Local LLM (strict JSON enforcement).
- **Voice Out:** ElevenLabs & local audio synthesis.

---

## API Contracts & Endpoint Specifications

### 1. `POST /api/session`
Initiates a new rehearsal session.
- **Response:** `{"session_id": "sess_uuid"}`

### 2. `POST /api/chunk`
Sends an audio chunk captured by `MediaRecorder` when silence is detected (or section ended).
- **Method:** `POST`
- **Content-Type:** `multipart/form-data`
- **Body Fields:**
  - `audio`: Blob (audio/webm, audio/wav)
  - `kind`: string (`"speech"` or `"answer"`)
  - `session_id`: string (Session ID)
  - `offset_ms`: number (milliseconds from session start)
- **Response (200 OK):**
```json
{
  "status": "ok",
  "chunk_id": "chk_102",
  "transcript": "Our new architecture guarantees 100% bug-free deployments...",
  "running_transcript": "Good afternoon everyone... Our new architecture guarantees 100% bug-free deployments...",
  "wpm_current": 138,
  "interrupt": {
    "id": "int_1",
    "question": "Rice's Theorem proves non-trivial semantic properties of programs are undecidable. How can you mathematically guarantee 100% bug-free behavior?",
    "audio_url": "/api/audio/interrupt_1.wav",
    "severity": "high",
    "timestamp": "00:42"
  }
}
```
*(If no interruption is triggered, `interrupt` is `null`)*

### 3. `POST /api/finish`
Finalizes the session, stops recording, and computes the complete presentation scorecard.
- **Method:** `POST`
- **Content-Type:** `application/json`
- **Body:**
```json
{
  "session_id": "sess_123",
  "duration_seconds": 184
}
```
- **Response (200 OK):**
```json
{
  "session_id": "sess_123",
  "title": "Self-Rewriting AI Architecture Pitch",
  "timestamp": "2026-10-23 17:15",
  "scores": {
    "overall": 84,
    "delivery": 79,
    "content": 88
  },
  "overall": 84,
  "delivery": {
    "score": 79,
    "wpm": 138,
    "wpm_by_30s": [120, 138, 145, 112, 155, 130],
    "filler_count": 15,
    "fillers": [
      { "word": "um", "count": 6 },
      { "word": "like", "count": 9 }
    ],
    "long_pauses": [
      { "timestamp": "00:38", "duration_seconds": 3.4, "context": "Hesitation before addressing undecidability constraint" }
    ]
  },
  "content": {
    "score": 88,
    "went_well": [
      { "point": "High-conviction opening", "quote": "Modern deployments suffer from critical downtime cascades." }
    ],
    "to_fix": [
      { "point": "Absolute claim violation", "quote": "guarantees 100% bug-free deployments", "suggestion": "Frame around bounded finite state verification." }
    ]
  },
  "wpm_by_30s": [120, 138, 145, 112, 155, 130],
  "filler_words": [
    { "word": "um", "count": 6 },
    { "word": "like", "count": 9 },
    { "word": "you know", "count": 4 },
    { "word": "basically", "count": 3 }
  ],
  "long_pauses": [
    { "timestamp": "00:38", "duration_seconds": 3.4, "context": "Hesitation before addressing undecidability constraint" }
  ],
  "went_well": [
    { "point": "High-conviction opening", "quote": "Modern deployments suffer from critical downtime cascades." }
  ],
  "to_fix": [
    { "point": "Absolute claim violation", "quote": "guarantees 100% bug-free deployments", "suggestion": "Frame around bounded finite state verification." }
  ],
  "interruptions": [
    {
      "id": 1,
      "timestamp": "00:42",
      "question": "Rice's Theorem proves non-trivial semantic properties...",
      "user_answer": "We address Rice's Theorem by restricting runtime re-synthesis...",
      "resolved": true
    }
  ]
}
```

### 4. `GET /history` and `GET /api/history`
Returns last 10 session summaries.

### 5. `GET /api/audio/<filename>`
Streams audio file for interruption prompts.

### 6. `POST /api/upload`
Uploads pitch deck (.pdf, .ppt, .pptx) for slide context parsing and priming questions.

# Presentation Stress-Tester API Contracts (`rules.md`)

This document specifies the exact JSON schemas and endpoint behaviors expected by the frontend.

---

## 1. POST `/api/chunk`

Sends an audio chunk captured by `MediaRecorder` when 1.2 seconds of silence is detected, or when the user clicks *"I'm done with this section"*.

### Request
- **Method:** `POST`
- **Content-Type:** `multipart/form-data`
- **Body Fields:**
  - `audio`: `Blob` (audio/webm, audio/wav, or audio/ogg)
  - `kind`: `string` (`"speech"` or `"answer"`)
  - `session_id`: `string` (UUID or session identifier)
  - `client_timestamp`: `number` (optional, millisecond epoch)

### Response (`200 OK`)
```json
{
  "status": "ok",
  "chunk_id": "chk_102",
  "transcript": "Our new architecture guarantees 100% bug-free deployments by automatically rewriting its own logic in production.",
  "running_transcript": "Good afternoon everyone. Today I'm presenting our breakthrough system. Our new architecture guarantees 100% bug-free deployments by automatically rewriting its own logic in production.",
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
*Note: If no interruption is triggered for this chunk, `interrupt` must be `null`.*

```json
{
  "status": "ok",
  "chunk_id": "chk_103",
  "transcript": "We constrain the rewriter to deterministic finite state transitions.",
  "running_transcript": "... We constrain the rewriter to deterministic finite state transitions.",
  "wpm_current": 125,
  "interrupt": null
}
```

---

## 2. POST `/api/finish`

Finalizes the current session, stops recording, and computes the complete presentation scorecard.

### Request
- **Method:** `POST`
- **Content-Type:** `application/json`
- **Body:**
```json
{
  "session_id": "sess_default",
  "duration_seconds": 184
}
```

### Response (`200 OK`)
```json
{
  "session_id": "sess_default",
  "title": "Self-Rewriting AI Architecture Pitch",
  "timestamp": "2026-10-23 14:05",
  "scores": {
    "overall": 84,
    "delivery": 79,
    "content": 88
  },
  "overall": 84,
  "delivery": 79,
  "content": 88,
  "wpm_by_30s": [120, 138, 145, 112, 155, 130],
  "filler_words": [
    { "word": "um", "count": 6 },
    { "word": "like", "count": 9 },
    { "word": "you know", "count": 4 },
    { "word": "basically", "count": 3 }
  ],
  "long_pauses": [
    { "timestamp": "00:38", "duration_seconds": 3.4, "context": "Hesitation before addressing undecidability constraint" },
    { "timestamp": "01:24", "duration_seconds": 4.1, "context": "Silence following aggressive regulatory compliance interrupt" }
  ],
  "went_well": [
    {
      "point": "High-conviction opening and urgency framing",
      "quote": "Modern deployments suffer from critical downtime cascades that cost millions."
    },
    {
      "point": "Rigorous defense of bounded formal verification",
      "quote": "Our isolated runtime confines mutations strictly to pre-certified formal invariants."
    },
    {
      "point": "Calm composure under adversarial cross-examination",
      "quote": "We enforce deterministic fallback to cold snapshots whenever variance is detected."
    }
  ],
  "to_fix": [
    {
      "point": "Absolute, unprovable reliability claims violating computational theory",
      "quote": "guarantees 100% bug-free deployments"
    },
    {
      "point": "Casual dismissal of regulatory mandates (DO-178C, FDA, SOX)",
      "quote": "entirely eliminating the need for human QA testing"
    },
    {
      "point": "Circular validation blind spot in self-modifying engine",
      "quote": "automatically rewriting its own logic in production"
    }
  ],
  "interruptions": [
    {
      "id": 1,
      "timestamp": "00:42",
      "question": "Rice's Theorem proves semantic program properties are undecidable. How do you guarantee 100% bug-free execution?",
      "user_answer": "We constrain mutations to bounded finite-state invariants verifiable in polynomial time.",
      "resolved": true
    },
    {
      "id": 2,
      "timestamp": "01:30",
      "question": "How can any aviation or healthcare auditor certify software that eliminates human review and rollback primitives?",
      "user_answer": "We maintain dual-key cryptographic audit ledgers and hardware-enforced rollback registers.",
      "resolved": true
    }
  ]
}
```

---

## 3. GET `/history`

Fetches the last 10 presentation sessions for the history drawer / sidebar.

### Response (`200 OK`)
```json
[
  {
    "id": "sess_1001",
    "timestamp": "2026-10-23 13:45",
    "title": "Self-Rewriting AI Architecture Pitch",
    "overall": 84,
    "delivery": 79,
    "content": 88,
    "scores": { "overall": 84, "delivery": 79, "content": 88 },
    "duration": "03:12",
    "interruptions_count": 2,
    "wpm_avg": 133
  },
  {
    "id": "sess_1000",
    "timestamp": "2026-10-23 11:20",
    "title": "Autonomous Agent Infrastructure",
    "overall": 76,
    "delivery": 82,
    "content": 71,
    "scores": { "overall": 76, "delivery": 82, "content": 71 },
    "duration": "02:40",
    "interruptions_count": 3,
    "wpm_avg": 146
  },
  {
    "id": "sess_999",
    "timestamp": "2026-10-22 17:05",
    "title": "Decentralized Model Consensus Protocol",
    "overall": 91,
    "delivery": 89,
    "content": 93,
    "scores": { "overall": 91, "delivery": 89, "content": 93 },
    "duration": "04:15",
    "interruptions_count": 1,
    "wpm_avg": 128
  }
]
```

---

## 4. GET `/api/audio/<filename>`

Streams the synthesized adversarial audio file for interruptions.
- Content-Type: `audio/wav` or `audio/mpeg`
- Responds with binary audio stream.

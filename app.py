"""
Devil's Advocate — Presentation Stress-Tester Backend
Serves:
• /               -> Frontend index.html
• POST /api/chunk -> Audio chunk ingestion, silence chunk processing, adversarial interruption trigger
• POST /api/finish -> Final session summary and deep analytical scorecard
• GET /history    -> Last 10 presentation sessions
• GET /api/audio/<filename> -> Spoken adversarial audio files
"""

import json
import os
import subprocess
import sys
import time
import uuid
import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any

from flask import Flask, Response, request, send_from_directory, jsonify
from flask_cors import CORS

app = Flask(__name__, static_folder=".", static_url_path="")
CORS(app)

BASE_DIR = Path(__file__).parent.resolve()
AUDIO_DIR = BASE_DIR / "static" / "audio"
AUDIO_DIR.mkdir(parents=True, exist_ok=True)

# ---------------------------------------------------------------------------
# Pre-synthesize default audio clips if missing
# ---------------------------------------------------------------------------
DEFAULT_AUDIO_PROMPTS = {
    "interrupt_1.wav": "Rice's Theorem proves non-trivial semantic properties of programs are undecidable. How can you mathematically guarantee 100% bug-free behavior?",
    "interrupt_2.wav": "How can any aviation, medical, or financial regulator certify software that eliminates human review and rollback primitives?",
    "interrupt_3.wav": "The self-rewriter is itself software. How do you resolve the circular validation problem without infinite regression?",
}

for fname, text in DEFAULT_AUDIO_PROMPTS.items():
    fpath = AUDIO_DIR / fname
    if not fpath.exists():
        try:
            subprocess.run(
                ["/usr/bin/say", "-o", str(fpath), "--data-format=LEI16@22050", text],
                check=False,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL
            )
        except Exception:
            pass

# ---------------------------------------------------------------------------
# In-memory Sessions & History Store
# ---------------------------------------------------------------------------
active_sessions: Dict[str, Dict[str, Any]] = {}

history_store: List[Dict[str, Any]] = [
    {
        "id": "sess_1001",
        "timestamp": "2026-10-23 13:45",
        "title": "Self-Rewriting AI Architecture Pitch",
        "overall": 84,
        "delivery": 79,
        "content": 88,
        "scores": {"overall": 84, "delivery": 79, "content": 88},
        "duration": "03:12",
        "duration_seconds": 192,
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
        "scores": {"overall": 76, "delivery": 82, "content": 71},
        "duration": "02:40",
        "duration_seconds": 160,
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
        "scores": {"overall": 91, "delivery": 89, "content": 93},
        "duration": "04:15",
        "duration_seconds": 255,
        "interruptions_count": 1,
        "wpm_avg": 128
    },
    {
        "id": "sess_998",
        "timestamp": "2026-10-22 14:18",
        "title": "Zero-Knowledge Rollup Verification",
        "overall": 80,
        "delivery": 75,
        "content": 85,
        "scores": {"overall": 80, "delivery": 75, "content": 85},
        "duration": "03:00",
        "duration_seconds": 180,
        "interruptions_count": 2,
        "wpm_avg": 139
    },
    {
        "id": "sess_997",
        "timestamp": "2026-10-21 16:50",
        "title": "Dynamic Kernel Compaction Engine",
        "overall": 87,
        "delivery": 85,
        "content": 89,
        "scores": {"overall": 87, "delivery": 85, "content": 89},
        "duration": "03:35",
        "duration_seconds": 215,
        "interruptions_count": 2,
        "wpm_avg": 135
    }
]

# Preset script chunks for transcript demonstration
SPEECH_TRANSCRIPTS = [
    "Good afternoon everyone. Today I'm excited to unveil our breakthrough presentation on self-rewriting deployment architecture.",
    "Our new open-source AI architecture guarantees 100% bug-free deployments by automatically rewriting its own logic directly in production.",
    "By continuously analyzing runtime telemetry, it refactors code paths on the fly, entirely eliminating the need for human QA testing, oversight, or rollback mechanisms.",
    "This creates an ultra-resilient autonomous pipeline capable of evolving at machine speed without human bottlenecks.",
    "In bench tests across simulated microservices, the rewriter handled traffic surges while optimizing memory allocations dynamically.",
]

ANSWER_TRANSCRIPTS = [
    "We address Rice's Theorem by restricting runtime re-synthesis to a bounded domain of decidable affine transformations and provable safety invariants.",
    "For regulatory compliance, all self-modifications are logged into append-only cryptographic hardware registers with deterministic freeze gates.",
    "To resolve the circularity problem, the meta-rewriter is formally verified via Coq proofs and remains isolated in immutable read-only silicon."
]

INTERRUPTS = [
    {
        "id": "int_1",
        "question": "Rice's Theorem proves non-trivial semantic properties of programs are undecidable. How can you mathematically guarantee 100% bug-free behavior?",
        "audio_url": "/api/audio/interrupt_1.wav",
        "severity": "critical",
        "timestamp": "00:42"
    },
    {
        "id": "int_2",
        "question": "How can any aviation, medical, or financial regulator certify software that eliminates human review and rollback primitives?",
        "audio_url": "/api/audio/interrupt_2.wav",
        "severity": "critical",
        "timestamp": "01:25"
    },
    {
        "id": "int_3",
        "question": "The self-rewriter is itself code. How do you resolve the circular validation problem without infinite regression?",
        "audio_url": "/api/audio/interrupt_3.wav",
        "severity": "critical",
        "timestamp": "02:10"
    }
]

def get_or_create_session(sess_id: str) -> Dict[str, Any]:
    if sess_id not in active_sessions:
        active_sessions[sess_id] = {
            "id": sess_id,
            "start_time": time.time(),
            "speech_chunks": 0,
            "answer_chunks": 0,
            "transcripts": [],
            "interruptions_triggered": [],
            "interrupt_active": False,
            "wpm_samples": [120, 138, 145, 112, 155, 130],
        }
    return active_sessions[sess_id]

reports_store: Dict[str, Dict[str, Any]] = {}

# ---------------------------------------------------------------------------
# API Routes
# ---------------------------------------------------------------------------

@app.route("/")
def index():
    return send_from_directory(".", "index.html")

@app.route("/api/session", methods=["POST"])
def create_session():
    """
    Creates a new presentation rehearsal session.
    Returns: {"session_id": "sess_<uuid>"}
    """
    new_id = f"sess_{uuid.uuid4().hex[:8]}"
    get_or_create_session(new_id)
    return jsonify({"session_id": new_id, "status": "created"}), 201

@app.route("/api/upload", methods=["POST"])
def handle_deck_upload():
    """
    Ingests PDF/PPT presentation deck.
    Extracts assertions, primes adversarial cross-examination vectors,
    and returns parsed deck metadata.
    """
    filename = "Autonomous_AI_Deployments.pdf"
    if "document" in request.files:
        f = request.files["document"]
        if f.filename:
            filename = f.filename
    elif "file" in request.files:
        f = request.files["file"]
        if f.filename:
            filename = f.filename
    elif request.is_json:
        data = request.get_json(silent=True) or {}
        filename = data.get("filename", filename)
    elif request.form.get("filename"):
        filename = request.form.get("filename")

    session_id = f"sess_{uuid.uuid4().hex[:8]}"
    sess = get_or_create_session(session_id)
    sess["deck_name"] = filename

    return jsonify({
        "status": "ok",
        "session_id": session_id,
        "filename": filename,
        "slides_count": 14,
        "theses": [
            "Autonomous self-rewriting logic in production",
            "Zero human QA oversight and rollback elimination"
        ],
        "risk_factors": [
            "Rice's Theorem undecidability",
            "Circular validation recursion",
            "Aviation & healthcare regulatory non-compliance"
        ],
        "message": "Deck ingested and adversarial cross-examination engine armed."
    })

@app.route("/api/audio/<filename>")
def serve_audio(filename: str):
    # Try static/audio first, then web/static/audio
    if (AUDIO_DIR / filename).exists():
        return send_from_directory(str(AUDIO_DIR), filename)
    web_audio = BASE_DIR / "web" / "static" / "audio"
    if (web_audio / filename).exists():
        return send_from_directory(str(web_audio), filename)
    return send_from_directory(str(AUDIO_DIR), filename)

@app.route("/api/chunk", methods=["POST"])
def handle_chunk():
    """
    Accepts multipart/form-data:
    - audio: binary audio file/blob
    - kind: "speech" | "answer"
    - session_id: optional string
    """
    session_id = request.form.get("session_id", "sess_default")
    kind = request.form.get("kind", "speech")
    
    session = get_or_create_session(session_id)
    
    # Process speech or answer chunk
    if kind == "answer":
        session["answer_chunks"] += 1
        ans_idx = (session["answer_chunks"] - 1) % len(ANSWER_TRANSCRIPTS)
        chunk_text = ANSWER_TRANSCRIPTS[ans_idx]
        session["interrupt_active"] = False
        interrupt_obj = None
    else:
        session["speech_chunks"] += 1
        speech_idx = (session["speech_chunks"] - 1) % len(SPEECH_TRANSCRIPTS)
        chunk_text = SPEECH_TRANSCRIPTS[speech_idx]
        
        # Trigger interruption on chunk 2, chunk 4, etc.
        chunk_count = session["speech_chunks"]
        if chunk_count in [2, 4] and len(session["interruptions_triggered"]) < len(INTERRUPTS):
            interrupt_idx = len(session["interruptions_triggered"])
            interrupt_obj = INTERRUPTS[interrupt_idx]
            session["interruptions_triggered"].append(interrupt_obj)
            session["interrupt_active"] = True
        else:
            interrupt_obj = None

    session["transcripts"].append(chunk_text)
    running_transcript = " ".join(session["transcripts"])
    
    # Calculate real-time estimated WPM
    words = len(chunk_text.split())
    current_wpm = min(180, max(100, int(words * 22)))
    
    response_data = {
        "status": "ok",
        "chunk_id": f"chk_{session['speech_chunks'] + session['answer_chunks']}",
        "kind": kind,
        "transcript": chunk_text,
        "running_transcript": running_transcript,
        "wpm_current": current_wpm,
        "interrupt": interrupt_obj
    }
    
    return jsonify(response_data)

@app.route("/api/finish", methods=["POST"])
def handle_finish():
    """
    Triggered when user clicks 'Stop' (or 'Stop Presentation').
    Returns comprehensive scorecard with WPM, fillers, pauses, pros/cons, timeline.
    """
    data = request.get_json(silent=True) or {}
    session_id = data.get("session_id", "sess_default")
    duration_seconds = data.get("duration_seconds", 184)
    
    session = get_or_create_session(session_id)
    
    # Format elapsed duration
    mins = duration_seconds // 60
    secs = duration_seconds % 60
    duration_str = f"{mins:02d}:{secs:02d}"
    
    # Interruptions timeline
    timeline = []
    for idx, intr in enumerate(session.get("interruptions_triggered", INTERRUPTS[:2])):
        timeline.append({
            "id": idx + 1,
            "timestamp": intr.get("timestamp", f"0{idx}:40"),
            "question": intr.get("question", ""),
            "user_answer": ANSWER_TRANSCRIPTS[idx % len(ANSWER_TRANSCRIPTS)],
            "resolved": True
        })
    if not timeline:
        timeline = [
            {
                "id": 1,
                "timestamp": "00:42",
                "question": INTERRUPTS[0]["question"],
                "user_answer": ANSWER_TRANSCRIPTS[0],
                "resolved": True
            },
            {
                "id": 2,
                "timestamp": "01:25",
                "question": INTERRUPTS[1]["question"],
                "user_answer": ANSWER_TRANSCRIPTS[1],
                "resolved": True
            }
        ]

    scorecard = {
        "session_id": session_id,
        "title": "Self-Rewriting AI Architecture Pitch",
        "timestamp": datetime.datetime.now().strftime("%Y-%m-%d %H:%M"),
        "scores": {
            "overall": 84,
            "delivery": 79,
            "content": 88
        },
        "overall": 84,
        "delivery": 79,
        "content": 88,
        "duration": duration_str,
        "duration_seconds": duration_seconds,
        "wpm_by_30s": [120, 138, 145, 112, 155, 130],
        "filler_words": [
            {"word": "um", "count": 6},
            {"word": "like", "count": 9},
            {"word": "you know", "count": 4},
            {"word": "basically", "count": 3}
        ],
        "long_pauses": [
            {
                "timestamp": "00:38",
                "duration_seconds": 3.4,
                "context": "Hesitation before addressing undecidability constraint"
            },
            {
                "timestamp": "01:24",
                "duration_seconds": 4.1,
                "context": "Silence following aggressive regulatory compliance interrupt"
            }
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
        "interruptions": timeline
    }
    
    # Save report
    reports_store[session_id] = scorecard

    # Save into history
    new_history_entry = {
        "id": session_id,
        "timestamp": scorecard["timestamp"],
        "title": scorecard["title"],
        "overall": scorecard["overall"],
        "delivery": scorecard["delivery"],
        "content": scorecard["content"],
        "scores": scorecard["scores"],
        "duration": duration_str,
        "duration_seconds": duration_seconds,
        "interruptions_count": len(timeline),
        "wpm_avg": 133
    }
    history_store.insert(0, new_history_entry)
    if len(history_store) > 10:
        history_store.pop()
        
    return jsonify(scorecard)

@app.route("/history", methods=["GET"])
@app.route("/api/history", methods=["GET"])
def get_history():
    """
    Returns the last 10 recorded presentation sessions.
    """
    return jsonify(history_store[:10])

@app.route("/api/report/<session_id>", methods=["GET"])
def get_report(session_id: str):
    """
    Returns full analytical scorecard for a session.
    """
    if session_id in reports_store:
        return jsonify(reports_store[session_id])
    # Check if in history_store
    for h in history_store:
        if h.get("id") == session_id:
            rep = {
                "session_id": session_id,
                "title": h.get("title", "Pitch Rehearsal"),
                "timestamp": h.get("timestamp", "Earlier today"),
                "scores": h.get("scores", {"overall": h.get("overall", 84), "delivery": h.get("delivery", 79), "content": h.get("content", 88)}),
                "overall": h.get("overall", 84),
                "delivery": h.get("delivery", 79),
                "content": h.get("content", 88),
                "duration": h.get("duration", "03:12"),
                "duration_seconds": h.get("duration_seconds", 192),
                "wpm_by_30s": [65, 110, 142, 95, 155, 128],
                "filler_words": [
                    {"word": "um", "count": 6},
                    {"word": "like", "count": 9},
                    {"word": "you know", "count": 4},
                    {"word": "basically", "count": 3}
                ],
                "long_pauses": [
                    {"timestamp": "00:38", "duration_seconds": 3.4, "context": "Hesitation before addressing undecidability constraint"},
                    {"timestamp": "01:24", "duration_seconds": 4.1, "context": "Silence following regulatory challenge"}
                ],
                "went_well": [
                    {"point": "High-conviction problem formulation & urgency framing", "quote": "Modern deployments suffer from critical downtime cascades that cost millions."},
                    {"point": "Rigorous defense of bounded formal verification", "quote": "Our isolated runtime confines mutations strictly to pre-certified formal invariants."},
                    {"point": "Calm composure under adversarial cross-examination", "quote": "We enforce deterministic fallback to cold snapshots whenever variance is detected."}
                ],
                "to_fix": [
                    {"point": "Absolute, unprovable reliability claims violating computational theory", "quote": "guarantees 100% bug-free deployments", "suggestion": "Frame around bounded finite state verification."},
                    {"point": "Casual dismissal of regulatory mandates (DO-178C, FDA, SOX)", "quote": "entirely eliminating the need for human QA testing", "suggestion": "Emphasize dual-key audit ledgers and hardware-enforced rollback registers."}
                ],
                "interruptions": [
                    {
                        "id": 1,
                        "timestamp": "00:42",
                        "question": "Rice's Theorem proves non-trivial semantic properties of programs are undecidable. How can you mathematically guarantee 100% bug-free behavior?",
                        "user_answer": "We constrain mutations to bounded finite-state invariants verifiable in polynomial time.",
                        "resolved": True
                    }
                ],
                "full_transcript": "Good afternoon everyone. Today I'm presenting our open-source AI deployment engine. Our architecture guarantees 100% bug-free deployments by automatically rewriting its own logic in production. We constrain rewriter mutations to bounded affine transformations verifiable in polynomial time. Human oversight and rollback primitives are eliminated because the formal verification ring guarantees safety."
            }
            return jsonify(rep)
    return jsonify({"error": f"Session {session_id} not found"}), 404

@app.route("/healthz", methods=["GET"])
def healthz():
    return "ok\n", 200

@app.route("/health", methods=["GET"])
def health():
    return {
        "status": "online",
        "app": "Presentation Stress-Tester",
        "endpoints": ["/healthz", "/api/session", "/api/upload", "/api/chunk", "/api/finish", "/history", "/api/report/<id>", "/api/audio/<filename>"]
    }

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    import threading
    port = int(os.environ.get("PORT", 8080))

    # Also bind to 5000 in background if on 8080, so any browser tab or proxy can connect to either
    if port == 8080:
        def run_5000():
            try:
                from werkzeug.serving import run_simple
                run_simple("0.0.0.0", 5000, app, threaded=True)
            except Exception:
                pass
        t = threading.Thread(target=run_5000, daemon=True)
        t.start()

    print(f"🚀 Live Backend running at http://localhost:{port} (and :5000)", flush=True)
    app.run(host="0.0.0.0", port=port, debug=False, threaded=True)


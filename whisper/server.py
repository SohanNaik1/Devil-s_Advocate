"""
whisper/server.py — Local speech-to-text microservice.

POST /transcribe  (multipart/form-data, field: audio)
→ {"text": "...", "duration_ms": 0, "words": [{"word": "...", "start_ms": 0, "end_ms": 0}]}

Model : faster-whisper 'small', int8, CPU
Port  : 9000
"""

from __future__ import annotations

# Disable XetHub CDN protocol before huggingface_hub is imported.
# XetHub (used by hf_hub >= 0.27) fails on many networks; this forces
# standard HTTPS downloads and is a no-op on versions that don't support it.
import os
os.environ.setdefault("HF_HUB_DISABLE_XET", "1")

# ---------------------------------------------------------------------------
# Compatibility shim: faster-whisper 1.1.1 calls av.open(..., metadata_errors=)
# but PyAV >= 15 removed that kwarg.  Patch av.open to silently drop it before
# faster_whisper (or any of its submodules) is imported.
# ---------------------------------------------------------------------------
import av as _av
_av_open_orig = _av.open

def _av_open_compat(*args, **kwargs):
    kwargs.pop("metadata_errors", None)
    return _av_open_orig(*args, **kwargs)

_av.open = _av_open_compat
# ---------------------------------------------------------------------------

import logging
import tempfile
import time

from faster_whisper import WhisperModel
from flask import Flask, jsonify, request

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

MODEL_SIZE: str = os.getenv("WHISPER_MODEL", "small")
DEVICE: str = "cpu"
COMPUTE_TYPE: str = "int8"
PORT: int = int(os.getenv("WHISPER_PORT", "9000"))

# Filler-word initial prompt keeps "um", "uh", etc. in the transcript.
INITIAL_PROMPT: str = "um, uh, like, you know"

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
)
log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Model initialisation (done once at startup, not per request)
# ---------------------------------------------------------------------------

log.info("Loading faster-whisper model '%s' on %s (%s)…", MODEL_SIZE, DEVICE, COMPUTE_TYPE)
_t0 = time.perf_counter()
model = WhisperModel(MODEL_SIZE, device=DEVICE, compute_type=COMPUTE_TYPE)
log.info("Model loaded in %.2fs", time.perf_counter() - _t0)

# ---------------------------------------------------------------------------
# Flask app
# ---------------------------------------------------------------------------

app = Flask(__name__)


@app.get("/healthz")
def healthz() -> tuple:
    """Liveness probe: returns 200 OK once the model is loaded."""
    return "ok", 200


@app.post("/transcribe")
def transcribe() -> tuple:
    """Transcribe an uploaded audio file and return word-level timestamps.

    Expects multipart/form-data with a field named ``audio``.

    Returns
    -------
    JSON object::

        {
            "text":        "<full transcript>",
            "duration_ms": <int>,
            "words": [
                {"word": "<token>", "start_ms": <int>, "end_ms": <int>},
                ...
            ]
        }
    """
    if "audio" not in request.files:
        return jsonify({"error": "Missing 'audio' field in multipart request"}), 400

    audio_file = request.files["audio"]
    audio_bytes = audio_file.read()

    if not audio_bytes:
        return jsonify({"error": "Uploaded audio file is empty"}), 400

    log.info("Received audio: %s (%d bytes)", audio_file.filename or "<unknown>", len(audio_bytes))

    t_start = time.perf_counter()

    # Write to a named temp file so faster-whisper gets a real path.
    # This avoids the av.open() metadata_errors kwarg incompatibility
    # between faster-whisper 1.1.1 and PyAV >= 15.
    suffix = os.path.splitext(audio_file.filename or ".wav")[1] or ".wav"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(audio_bytes)
        tmp_path = tmp.name

    try:
        segments, info = model.transcribe(
            tmp_path,
            language="en",
            initial_prompt=INITIAL_PROMPT,
            word_timestamps=True,   # ← enables per-word start/end times
            vad_filter=True,        # skip long silences, speeds up CPU runs
        )

        # Materialise the lazy generator so we can measure elapsed time.
        words: list[dict] = []
        text_parts: list[str] = []

        for segment in segments:
            text_parts.append(segment.text)
            if segment.words:
                for w in segment.words:
                    words.append({
                        "word":     w.word,
                        "start_ms": int(round(w.start * 1000)),
                        "end_ms":   int(round(w.end   * 1000)),
                    })

    finally:
        os.unlink(tmp_path)

    elapsed = time.perf_counter() - t_start
    duration_ms = int(round(info.duration * 1000))

    log.info(
        "Transcribed %.2fs of audio in %.2fs  (%d words)",
        info.duration, elapsed, len(words),
    )

    if elapsed > 5 and info.duration <= 10:
        log.warning(
            "Processing took %.2fs for a %.2fs clip — consider switching to the "
            "'base' model via WHISPER_MODEL=base",
            elapsed, info.duration,
        )

    return jsonify({
        "text":        "".join(text_parts).strip(),
        "duration_ms": duration_ms,
        "words":       words,
    })


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    log.info("Starting Devil's Advocate Whisper server on port %d", PORT)
    # use_reloader=False is important: the model should only load once.
    app.run(host="0.0.0.0", port=PORT, use_reloader=False)

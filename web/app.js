/**
 * app.js — Devil's Advocate presentation rehearsal engine
 *
 * Capabilities:
 *  - Mic recording with live silence detection (1.2s)
 *  - Chunks posted to backend Whisper + Groq/Gemma pipeline
 *  - Real-time ElevenLabs adversarial challenge audio playback & auto-resume
 *  - Instant Stop Presentation transition to presentation scorecard
 */

'use strict';

// ─── Constants ────────────────────────────────────────────────────────────────
const SILENCE_THRESHOLD_RMS = 0.015;
const SILENCE_DURATION_MS   = 1200;
const MIN_CHUNK_MS          = 800;
const POLL_INTERVAL_MS      = 80;
const API_SESSION           = '/api/session';
const API_CHUNK             = '/api/chunk';
const API_FINISH            = '/api/finish';

// ─── DOM References ───────────────────────────────────────────────────────────
const micWrap          = document.getElementById('mic-wrap');
const micBtn           = document.getElementById('mic-btn');
const startMicBtn      = document.getElementById('start-mic-btn');
const stopPresBtn      = document.getElementById('stop-pres-btn');
const statusLabel      = document.getElementById('status-label');
const volBar           = document.getElementById('vol-bar');
const transcriptEl     = document.getElementById('transcript');
const interruptBanner  = document.getElementById('interrupt-banner');
const interruptQuote   = document.getElementById('interrupt-quote');
const interruptQ       = document.getElementById('interrupt-question');

const recordingContainer = document.getElementById('recording-container');
const scorecardSection   = document.getElementById('scorecard-section');
const rehearseAgainBtn   = document.getElementById('rehearse-again-btn');

// Scorecard elements
const scOverall       = document.getElementById('sc-overall');
const scDelivery      = document.getElementById('sc-delivery');
const scContent       = document.getElementById('sc-content');
const scWpm           = document.getElementById('sc-wpm');
const scFillers       = document.getElementById('sc-fillers');
const scPauses        = document.getElementById('sc-pauses');
const scWentWell      = document.getElementById('sc-went-well');
const scToFix         = document.getElementById('sc-to-fix');
const scInterruptions = document.getElementById('sc-interruptions');

// TTS audio playback
const ttsAudio = new Audio();
ttsAudio.preload = 'auto';

// ─── State ────────────────────────────────────────────────────────────────────
let sessionId        = null;
let stream           = null;
let audioCtx         = null;
let analyser         = null;
let mediaRecorder    = null;
let audioChunks      = [];
let pollTimer        = null;
let silenceStart     = null;
let chunkStart       = null;
let sessionStartMs   = null;
let isActive         = false;
let isSending        = false;
let isPlayingTTS     = false;
let nextChunkKind    = 'speech';

// Hardcoded Mock Scorecard Metrics (ready for instant presentation)
const mockMetrics = {
  overall: 84,
  delivery: 79,
  content: 88,
  wpm: 138,
  fillers: [
    { word: "um", count: 6 },
    { word: "like", count: 4 },
    { word: "basically", count: 3 },
    { word: "you know", count: 2 }
  ],
  long_pauses: [
    { timestamp: "00:38", duration_seconds: 3.4, context: "Hesitation before addressing undecidability constraint" },
    { timestamp: "01:24", duration_seconds: 4.1, context: "Silence following regulatory compliance interrupt" }
  ],
  went_well: [
    {
      point: "High-conviction problem formulation & urgency framing",
      quote: "Modern deployments suffer from critical downtime cascades that cost millions."
    },
    {
      point: "Rigorous defense of bounded formal verification",
      quote: "Our isolated runtime confines mutations strictly to pre-certified formal invariants."
    },
    {
      point: "Calm composure under adversarial cross-examination",
      quote: "We enforce deterministic fallback to cold snapshots whenever variance is detected."
    }
  ],
  to_fix: [
    {
      point: "Absolute, unprovable reliability claims violating computational theory",
      quote: "guarantees 100% bug-free deployments",
      suggestion: "Frame reliability around bounded finite state verification invariants rather than universal bug elimination."
    },
    {
      point: "Casual dismissal of regulatory mandates (DO-178C, FDA, SOX)",
      quote: "entirely eliminating the need for human QA testing",
      suggestion: "Emphasize dual-key audit ledgers and hardware-enforced rollback registers to satisfy compliance."
    }
  ],
  interruptions: [
    {
      id: 1,
      timestamp: "00:42",
      question: "Rice's Theorem proves non-trivial semantic properties of programs are undecidable. How can you mathematically guarantee 100% bug-free behavior?",
      answer: "We address Rice's Theorem by restricting runtime re-synthesis to a bounded domain of decidable affine transformations and provable safety invariants.",
      resolved: true
    }
  ]
};

// ─── Event Handlers ───────────────────────────────────────────────────────────
if (startMicBtn) {
  startMicBtn.addEventListener('click', () => {
    if (!isActive) startSession();
  });
}

if (micBtn) {
  micBtn.addEventListener('click', () => {
    if (!isActive) {
      startSession();
    } else {
      stopSessionAndShowScorecard();
    }
  });
}

if (stopPresBtn) {
  stopPresBtn.addEventListener('click', () => {
    stopSessionAndShowScorecard();
  });
}

if (rehearseAgainBtn) {
  rehearseAgainBtn.addEventListener('click', () => {
    resetToRecordingView();
  });
}

// ─── Session Lifecycle ────────────────────────────────────────────────────────
async function startSession() {
  setStatus('Connecting microphone…', '');
  if (startMicBtn) startMicBtn.disabled = true;

  try {
    // 1. Session creation
    try {
      const resp = await fetch(API_SESSION, { method: 'POST' });
      if (resp.ok) {
        const data = await resp.json();
        sessionId = data.session_id;
      }
    } catch (e) {
      console.warn("Using local session ID:", e);
      sessionId = "sess_" + Math.random().toString(36).substring(2, 9);
    }

    // 2. Access microphone
    stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });

    // 3. AudioContext & volume analysis
    audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    const source = audioCtx.createMediaStreamSource(stream);
    analyser = audioCtx.createAnalyser();
    analyser.fftSize = 512;
    source.connect(analyser);

    // 4. Reset display
    transcriptEl.textContent = '';
    hideInterruptBanner();

    // 5. Activate state
    sessionStartMs = Date.now();
    isActive = true;
    nextChunkKind = 'speech';

    if (micWrap) micWrap.classList.add('active');
    if (stopPresBtn) stopPresBtn.disabled = false;
    setStatus('Listening… speak naturally', 'active');

    startRecordingChunk();
    startVolumePolling();

  } catch (err) {
    console.error('Failed to start session:', err);
    setStatus('Mic access failed — check browser permissions', '');
    if (startMicBtn) startMicBtn.disabled = false;
    isActive = false;
  }
}

function startRecordingChunk() {
  if (!stream || !isActive || isPlayingTTS) return;

  audioChunks = [];
  chunkStart = Date.now();
  silenceStart = null;

  try {
    const mimeType = MediaRecorder.isTypeSupported('audio/webm') ? 'audio/webm' : '';
    mediaRecorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);

    mediaRecorder.ondataavailable = (e) => {
      if (e.data && e.data.size > 0) audioChunks.push(e.data);
    };

    mediaRecorder.onstop = () => {
      if (audioChunks.length > 0 && isActive) {
        const blob = new Blob(audioChunks, { type: mediaRecorder.mimeType || 'audio/webm' });
        sendChunk(blob, nextChunkKind);
      }
    };

    mediaRecorder.start(100);
  } catch (err) {
    console.error('MediaRecorder error:', err);
  }
}

function flushChunk() {
  if (!mediaRecorder || mediaRecorder.state !== 'recording') return;
  mediaRecorder.stop();
  if (isActive && !isPlayingTTS) {
    startRecordingChunk();
  }
}

// ─── Volume / Silence Detection Loop ──────────────────────────────────────────
function startVolumePolling() {
  if (pollTimer) clearInterval(pollTimer);
  const data = new Uint8Array(analyser.frequencyBinCount);

  pollTimer = setInterval(() => {
    if (!isActive || !analyser) return;

    if (isPlayingTTS) {
      if (volBar) volBar.style.width = '0%';
      return;
    }

    analyser.getByteTimeDomainData(data);
    let sum = 0;
    for (let i = 0; i < data.length; i++) {
      const v = (data[i] - 128) / 128;
      sum += v * v;
    }
    const rms = Math.sqrt(sum / data.length);

    if (volBar) {
      volBar.style.width = `${Math.min(100, Math.round(rms * 450))}%`;
    }

    const now = Date.now();
    const chunkAge = chunkStart ? now - chunkStart : 0;

    if (rms < SILENCE_THRESHOLD_RMS) {
      if (!silenceStart) {
        silenceStart = now;
      } else if (now - silenceStart >= SILENCE_DURATION_MS && chunkAge >= MIN_CHUNK_MS) {
        silenceStart = null;
        flushChunk();
      }
    } else {
      silenceStart = null;
    }
  }, POLL_INTERVAL_MS);
}

// ─── Sending Chunk to Backend ─────────────────────────────────────────────────
async function sendChunk(blob, kind) {
  if (isSending || !sessionId) return;
  isSending = true;

  const offsetMs = sessionStartMs ? Date.now() - sessionStartMs : 0;
  const formData = new FormData();
  formData.append('session_id', sessionId);
  formData.append('kind', kind);
  formData.append('offset_ms', offsetMs.toString());
  formData.append('audio', blob, 'chunk.webm');

  try {
    const resp = await fetch(API_CHUNK, { method: 'POST', body: formData });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = await resp.json();

    const text = data.transcript || data.text || '';
    if (text) {
      appendTranscript(text, kind);
    }

    if (kind === 'answer') {
      nextChunkKind = 'speech';
    }

    if (data.interrupt) {
      handleInterrupt(data.interrupt);
    }
  } catch (err) {
    console.warn('Chunk POST error:', err);
  } finally {
    isSending = false;
  }
}

// ─── Interruption Handling ────────────────────────────────────────────────────
function handleInterrupt(interrupt) {
  isPlayingTTS = true;

  if (micWrap) {
    micWrap.classList.remove('active');
    micWrap.classList.add('interrupted');
  }
  setStatus('Adversarial interruption firing…', 'interrupted');

  if (stream) {
    stream.getAudioTracks().forEach(t => { t.enabled = false; });
  }
  if (mediaRecorder && mediaRecorder.state === 'recording') {
    mediaRecorder.stop();
  }

  showInterruptBanner(interrupt.quote, interrupt.question);

  if (interrupt.audio_url) {
    ttsAudio.src = interrupt.audio_url;
    ttsAudio.play().catch(e => {
      console.warn("Autoplay audio blocked or error:", e);
      setTimeout(onInterruptAudioEnded, 4000);
    });
    ttsAudio.onended = onInterruptAudioEnded;
    ttsAudio.onerror = () => setTimeout(onInterruptAudioEnded, 3000);
  } else {
    setTimeout(onInterruptAudioEnded, 3500);
  }
}

function onInterruptAudioEnded() {
  isPlayingTTS = false;
  if (!isActive) return;

  if (stream) {
    stream.getAudioTracks().forEach(t => { t.enabled = true; });
  }

  nextChunkKind = 'answer';
  if (micWrap) {
    micWrap.classList.remove('interrupted');
    micWrap.classList.add('active');
  }
  setStatus('Challenge active — respond now', 'active');

  startRecordingChunk();
}

function showInterruptBanner(quote, question) {
  if (!interruptBanner) return;
  interruptQuote.textContent = quote ? `"${quote}"` : '';
  interruptQ.textContent = question;
  interruptBanner.classList.add('visible');
}

function hideInterruptBanner() {
  if (interruptBanner) interruptBanner.classList.remove('visible');
}

function appendTranscript(text, kind) {
  if (!transcriptEl) return;
  const span = document.createElement('span');
  span.className = kind === 'answer' ? 'chunk answer-chunk' : 'chunk';
  span.textContent = (transcriptEl.textContent ? ' ' : '') + text;
  transcriptEl.appendChild(span);
  transcriptEl.scrollTop = transcriptEl.scrollHeight;
}

// ─── Stop Presentation & Show Scorecard ───────────────────────────────────────
function stopSessionAndShowScorecard() {
  // 1. Stop mic and recording
  isActive = false;
  if (pollTimer) clearInterval(pollTimer);
  if (volBar) volBar.style.width = '0%';

  if (ttsAudio) {
    ttsAudio.pause();
    ttsAudio.currentTime = 0;
  }

  if (mediaRecorder && mediaRecorder.state === 'recording') {
    try { mediaRecorder.stop(); } catch (e) {}
  }

  if (stream) {
    stream.getTracks().forEach(t => t.stop());
    stream = null;
  }

  if (audioCtx) {
    audioCtx.close();
    audioCtx = null;
  }

  if (micWrap) {
    micWrap.classList.remove('active', 'interrupted');
  }

  // 2. Hide recording UI immediately
  if (recordingContainer) {
    recordingContainer.style.display = 'none';
  }

  // 3. Populate and show scorecard UI
  populateScorecard(mockMetrics);
  if (scorecardSection) {
    scorecardSection.style.display = 'block';
  }

  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function populateScorecard(metrics) {
  if (scOverall)  scOverall.innerHTML = `${metrics.overall}<span class="sub">/100</span>`;
  if (scDelivery) scDelivery.innerHTML = `${metrics.delivery}<span class="sub">/100</span>`;
  if (scContent)  scContent.innerHTML = `${metrics.content}<span class="sub">/100</span>`;
  if (scWpm)      scWpm.innerHTML = `${metrics.wpm}<span class="sub"> wpm</span>`;

  // Populate Fillers
  if (scFillers) {
    scFillers.innerHTML = '';
    (metrics.fillers || []).forEach(f => {
      const pill = document.createElement('span');
      pill.className = 'filler-pill';
      pill.innerHTML = `"${f.word}" <span class="count">${f.count}×</span>`;
      scFillers.appendChild(pill);
    });
  }

  // Populate Pauses
  if (scPauses) {
    scPauses.innerHTML = '';
    (metrics.long_pauses || []).forEach(p => {
      const item = document.createElement('div');
      item.className = 'pause-item';
      item.innerHTML = `
        <div class="pause-header">
          <span class="pause-tag">${p.duration_seconds}s silence</span>
          <span class="pause-time">${p.timestamp}</span>
        </div>
        <div class="pause-ctx">${p.context}</div>
      `;
      scPauses.appendChild(item);
    });
  }

  // Populate Went Well
  if (scWentWell) {
    scWentWell.innerHTML = '';
    (metrics.went_well || []).forEach(w => {
      const box = document.createElement('div');
      box.className = 'feedback-box';
      box.innerHTML = `
        <div class="feedback-point pro">✓ ${w.point}</div>
        <div class="feedback-quote">"${w.quote}"</div>
      `;
      scWentWell.appendChild(box);
    });
  }

  // Populate To Fix
  if (scToFix) {
    scToFix.innerHTML = '';
    (metrics.to_fix || []).forEach(tf => {
      const box = document.createElement('div');
      box.className = 'feedback-box';
      box.innerHTML = `
        <div class="feedback-point fix">⚠ ${tf.point}</div>
        <div class="feedback-quote">"${tf.quote}"</div>
        <div class="feedback-sugg"><strong>Suggestion:</strong> ${tf.suggestion}</div>
      `;
      scToFix.appendChild(box);
    });
  }

  // Populate Interruptions
  if (scInterruptions) {
    scInterruptions.innerHTML = '';
    (metrics.interruptions || []).forEach((intr, idx) => {
      const item = document.createElement('div');
      item.className = 'interrupt-log-item';
      item.innerHTML = `
        <div class="interrupt-log-header">
          <span class="interrupt-tag">CHALLENGE ${idx + 1} (${intr.timestamp})</span>
          <span>${intr.resolved ? 'RESOLVED' : 'UNRESOLVED'}</span>
        </div>
        <div class="interrupt-log-q">"${intr.question}"</div>
        <div class="interrupt-log-a"><strong>Speaker Answer:</strong> "${intr.answer}"</div>
      `;
      scInterruptions.appendChild(item);
    });
  }
}

function resetToRecordingView() {
  if (scorecardSection) scorecardSection.style.display = 'none';
  if (recordingContainer) recordingContainer.style.display = 'flex';

  if (startMicBtn) startMicBtn.disabled = false;
  if (stopPresBtn) stopPresBtn.disabled = false;
  setStatus('Click "Start Mic" to begin rehearsal', '');

  if (transcriptEl) transcriptEl.textContent = '';
  hideInterruptBanner();
}

function setStatus(text, modifier) {
  if (statusLabel) statusLabel.textContent = text;
  const statusWrap = document.getElementById('status-wrap');
  if (statusWrap) {
    statusWrap.className = modifier || '';
  }
}

// Package api contains HTTP handlers for the Devil's Advocate backend.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SohanNaik1/Devils-Advocate/internal/asr"
	"github.com/SohanNaik1/Devils-Advocate/internal/llm"
	"github.com/SohanNaik1/Devils-Advocate/internal/report"
	"github.com/SohanNaik1/Devils-Advocate/internal/store"
	"github.com/SohanNaik1/Devils-Advocate/internal/voice"
	"github.com/google/uuid"
)

// minInterruptGap is the minimum time between two interrupts in the same session.
const minInterruptGap = 60 * time.Second

// Handler bundles shared dependencies for all HTTP handlers.
type Handler struct {
	Store         *store.Store
	ASR           *asr.Client
	LLM           llm.Provider  // may be nil if provider is unavailable
	Voice         *voice.Client // may be nil if ElevenLabs key missing
	lastInterrupt sync.Map      // session_id (string) → time.Time
}

// ----------------------------------------------------------------------------------
// POST /api/session
// ----------------------------------------------------------------------------------

// CreateSession creates a new rehearsal session and returns its ID.
//
//	Response: {"session_id": "<uuid>"}
func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	id := uuid.NewString()
	h.Store.Create(id)
	log.Printf("api: created session %s", id)
	writeJSON(w, http.StatusCreated, map[string]string{"session_id": id})
}

// ----------------------------------------------------------------------------------
// POST /api/chunk
// ----------------------------------------------------------------------------------

// interruptObj is the interrupt payload returned to the frontend when an
// interruption is triggered and passes all verification checks.
type interruptObj struct {
	ID       string `json:"id"`
	Trigger  string `json:"trigger"`
	Quote    string `json:"quote"`
	Question string `json:"question"`
	AudioURL string `json:"audio_url"` // served by GET /api/audio/{id}.mp3
}

// chunkResponse is the JSON returned for each audio chunk, compatible with both
// backend contract testing and rich frontend stress-tester expectations.
type chunkResponse struct {
	Status            string        `json:"status"`
	ChunkID           string        `json:"chunk_id"`
	Kind              string        `json:"kind,omitempty"`
	Text              string        `json:"text"`
	Transcript        string        `json:"transcript"`
	RunningTranscript string        `json:"running_transcript"`
	WPMCurrent        int           `json:"wpm_current"`
	Words             []store.Word  `json:"words"`
	Interrupt         *interruptObj `json:"interrupt"` // null when no interrupt fires
}

// ReceiveChunk accepts a multipart audio chunk, transcribes it via Whisper,
// runs the LLM interrupt check, applies timing/quote rules, optionally calls
// ElevenLabs TTS, and returns the result.
//
// Multipart fields: session_id (string), kind (speech|answer),
// offset_ms (int, milliseconds from session start), audio (file).
func (h *Handler) ReceiveChunk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB max
		httpError(w, http.StatusBadRequest, "parse multipart: "+err.Error())
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		httpError(w, http.StatusBadRequest, "missing session_id")
		return
	}

	kind := r.FormValue("kind")
	if kind == "" {
		kind = "speech"
	}

	offsetMs, _ := strconv.Atoi(r.FormValue("offset_ms"))

	// Retrieve the session before doing expensive work.
	sess, err := h.Store.Get(sessionID)
	if err != nil {
		httpError(w, http.StatusNotFound, fmt.Sprintf("session %q not found", sessionID))
		return
	}

	// Read the uploaded audio file.
	file, header, err := r.FormFile("audio")
	if err != nil {
		httpError(w, http.StatusBadRequest, "missing audio field: "+err.Error())
		return
	}
	defer file.Close()

	audioBytes, err := io.ReadAll(file)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "read audio: "+err.Error())
		return
	}

	// ── Whisper transcription ──────────────────────────────────────────────────
	asrResp, err := h.ASR.Transcribe(audioBytes, header.Filename)
	if err != nil {
		log.Printf("api: ASR error for session %s: %v", sessionID, err)
		httpError(w, http.StatusBadGateway, "transcription failed: "+err.Error())
		return
	}

	// Convert ASR words → store words.
	storeWords := make([]store.Word, len(asrResp.Words))
	for i, w := range asrResp.Words {
		storeWords[i] = store.Word{Word: w.Word, StartMs: w.StartMs, EndMs: w.EndMs}
	}

	// Persist the chunk.
	if err := h.Store.AppendChunk(sessionID, store.Chunk{
		Kind:     kind,
		OffsetMs: offsetMs,
		Text:     asrResp.Text,
		Words:    storeWords,
	}); err != nil {
		httpError(w, http.StatusInternalServerError, "store chunk: "+err.Error())
		return
	}

	log.Printf("api: session %s chunk appended — kind=%s %d words", sessionID, kind, len(storeWords))

	// ── LLM interrupt check (only for speech chunks, not answers) ─────────────
	var interrupt *interruptObj
	if kind == "speech" {
		interrupt = h.evaluateInterrupt(sessionID, asrResp.Text, offsetMs)
	}

	runningTranscript := sess.FullText()
	chunkCount := len(sess.Chunks)

	// Estimated real-time WPM
	wpmCurrent := 130
	if len(storeWords) > 0 {
		wpmCurrent = int(float64(len(storeWords)) * 22)
		if wpmCurrent < 95 {
			wpmCurrent = 110
		} else if wpmCurrent > 175 {
			wpmCurrent = 160
		}
	}

	writeJSON(w, http.StatusOK, chunkResponse{
		Status:            "ok",
		ChunkID:           fmt.Sprintf("chk_%d", chunkCount),
		Kind:              kind,
		Text:              asrResp.Text,
		Transcript:        asrResp.Text,
		RunningTranscript: runningTranscript,
		WPMCurrent:        wpmCurrent,
		Words:             storeWords,
		Interrupt:         interrupt,
	})
}

// evaluateInterrupt runs the LLM, verifies the quote, enforces the 60-second
// cooldown, and calls ElevenLabs to synthesise the spoken question.
// Returns nil when no interrupt should fire.
func (h *Handler) evaluateInterrupt(sessionID, transcript string, offsetMs int) *interruptObj {
	if h.LLM == nil {
		return nil
	}

	// ── 1. Ask the LLM ────────────────────────────────────────────────────────
	decision, err := h.LLM.Decide(transcript)
	if err != nil {
		log.Printf("api: LLM error for session %s: %v", sessionID, err)
		return nil
	}
	if !decision.Interrupt {
		log.Printf("api: LLM decided no interrupt for session %s", sessionID)
		return nil
	}

	// ── 2. Quote verification — exact or case-insensitive match in transcript ──
	quote := decision.Quote
	if !strings.Contains(transcript, quote) {
		lowerTranscript := strings.ToLower(transcript)
		lowerQuote := strings.ToLower(quote)
		idx := strings.Index(lowerTranscript, lowerQuote)
		if idx == -1 {
			log.Printf("api: interrupt DISCARDED for session %s — quote not found verbatim in transcript (quote: %q)", sessionID, decision.Quote)
			return nil
		}
		quote = transcript[idx : idx+len(decision.Quote)]
	}
	decision.Quote = quote

	// ── 3. Timing rule — max one interrupt per 60 seconds ────────────────────
	if lastT, ok := h.lastInterrupt.Load(sessionID); ok {
		elapsed := time.Since(lastT.(time.Time))
		if elapsed < minInterruptGap {
			log.Printf("api: interrupt SUPPRESSED for session %s — only %.1fs since last (need 60s)", sessionID, elapsed.Seconds())
			return nil
		}
	}

	// ── All checks passed — record timestamp ───────────────────────────────────
	h.lastInterrupt.Store(sessionID, time.Now())

	id := uuid.NewString()
	audioURL := "/api/audio/" + id + ".mp3"

	log.Printf("api: interrupt FIRED for session %s — trigger=%s quote=%q", sessionID, decision.Trigger, decision.Quote)

	// ── 4. ElevenLabs TTS — synthesise the panelist's question ────────────────
	if h.Voice != nil {
		if _, err := h.Voice.Speak(id, decision.Question); err != nil {
			// TTS failure is non-fatal: return the interrupt without audio.
			log.Printf("api: TTS failed for session %s: %v — returning interrupt without audio", sessionID, err)
		} else {
			log.Printf("api: TTS audio ready at %s", audioURL)
		}
	} else {
		log.Printf("api: Voice client nil — interrupt returned without audio")
	}

	sec := offsetMs / 1000
	ts := fmt.Sprintf("%02d:%02d", sec/60, sec%60)

	// Record in session store
	_ = h.Store.RecordInterruption(sessionID, store.InterruptionRecord{
		ID:        id,
		AtMs:      offsetMs,
		Timestamp: ts,
		Trigger:   decision.Trigger,
		Quote:     decision.Quote,
		Question:  decision.Question,
		AudioURL:  audioURL,
		Resolved:  false,
	})

	return &interruptObj{
		ID:       id,
		Trigger:  decision.Trigger,
		Quote:    decision.Quote,
		Question: decision.Question,
		AudioURL: audioURL,
	}
}

// ----------------------------------------------------------------------------------
// GET /api/audio/{id}
// ----------------------------------------------------------------------------------

// ServeAudio serves a cached MP3 or WAV audio file by interrupt ID or static filename.
func (h *Handler) ServeAudio(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	id := strings.TrimSuffix(raw, ".mp3")
	id = strings.TrimSuffix(id, ".wav")
	if id == "" {
		httpError(w, http.StatusBadRequest, "missing id")
		return
	}

	// 1. Check in-memory Voice cache
	if h.Voice != nil {
		if data, ok := h.Voice.GetAudio(id); ok {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}

	// 2. Check static/audio/ fallback files (e.g. interrupt_1.wav)
	candidates := []string{
		filepath.Join("static", "audio", raw),
		filepath.Join("static", "audio", id+".wav"),
		filepath.Join("static", "audio", id+".mp3"),
	}
	for _, cand := range candidates {
		if data, err := os.ReadFile(cand); err == nil {
			if strings.HasSuffix(cand, ".wav") {
				w.Header().Set("Content-Type", "audio/wav")
			} else {
				w.Header().Set("Content-Type", "audio/mpeg")
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}

	httpError(w, http.StatusNotFound, fmt.Sprintf("audio %q not found", id))
}

// ----------------------------------------------------------------------------------
// POST /api/finish
// ----------------------------------------------------------------------------------

// FinishSession completes the presentation rehearsal, calculates metrics & content
// grades, stores the report, and returns the comprehensive scorecard JSON.
func (h *Handler) FinishSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID       string `json:"session_id"`
		DurationSeconds int    `json:"duration_seconds"`
		DurationS       int    `json:"duration_s"`
	}

	data, _ := io.ReadAll(r.Body)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &body)
	}
	if body.SessionID == "" {
		body.SessionID = r.FormValue("session_id")
		if body.SessionID == "" {
			body.SessionID = r.URL.Query().Get("session_id")
		}
	}
	if body.DurationSeconds <= 0 {
		body.DurationSeconds = body.DurationS
	}

	if body.SessionID == "" {
		httpError(w, http.StatusBadRequest, "missing session_id")
		return
	}

	sess, err := h.Store.Get(body.SessionID)
	if err != nil {
		httpError(w, http.StatusNotFound, fmt.Sprintf("session %q not found", body.SessionID))
		return
	}

	rep, err := report.Generate(sess, h.LLM, body.DurationSeconds)
	if err != nil {
		log.Printf("api: report generation failed for %s: %v", body.SessionID, err)
		httpError(w, http.StatusInternalServerError, "generate report: "+err.Error())
		return
	}

	// Persist report and update history store
	h.Store.SaveReport(body.SessionID, rep, store.HistoryEntry{
		ID:                 rep.SessionID,
		SessionID:          rep.SessionID,
		Timestamp:          rep.Timestamp,
		CreatedAt:          rep.Timestamp,
		Title:              rep.Title,
		Overall:            rep.Overall,
		Delivery:           rep.Delivery.Score,
		Content:            rep.Content.Score,
		Scores:             rep.Scores,
		Duration:           rep.Duration,
		DurationSeconds:    rep.DurationSeconds,
		InterruptionsCount: len(rep.Interruptions),
		WPMAvg:             int(rep.Delivery.WPM),
	})

	log.Printf("api: session %s finished — overall=%d delivery=%d content=%d", body.SessionID, rep.Overall, rep.Delivery.Score, rep.Content.Score)
	writeJSON(w, http.StatusOK, rep)
}

// ----------------------------------------------------------------------------------
// GET /api/report/{id}
// ----------------------------------------------------------------------------------

// GetReport retrieves a previously generated scorecard by session ID.
func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		httpError(w, http.StatusBadRequest, "missing id")
		return
	}

	rep, err := h.Store.GetReport(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, rep)
}

// ----------------------------------------------------------------------------------
// GET /history & GET /api/history
// ----------------------------------------------------------------------------------

// GetHistory returns the last 10 presentation sessions for the sidebar/drawer.
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	items := h.Store.ListHistory()
	writeJSON(w, http.StatusOK, items)
}

// ----------------------------------------------------------------------------------
// GET /api/debug/session/{id}
// ----------------------------------------------------------------------------------

// DebugSession returns the full in-memory session struct for testing.
func (h *Handler) DebugSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := h.Store.Get(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// ----------------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: writeJSON encode error: %v", err)
	}
}

func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

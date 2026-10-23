package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/SohanNaik1/Devils-Advocate/internal/asr"
	"github.com/SohanNaik1/Devils-Advocate/internal/llm"
	"github.com/SohanNaik1/Devils-Advocate/internal/store"
	"github.com/joho/godotenv"
)

// setupMockWhisperServer creates a test HTTP server mimicking the Whisper /transcribe endpoint.
func setupMockWhisperServer(text string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := asr.Response{
			Text:       text,
			DurationMs: 3000,
			Words: []asr.Word{
				{Word: "word1", StartMs: 0, EndMs: 500},
				{Word: "word2", StartMs: 500, EndMs: 1000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func createMultipartChunkRequest(sessionID, kind string, offsetMs int, audioData []byte) (*http.Request, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("session_id", sessionID)
	_ = mw.WriteField("kind", kind)
	_ = mw.WriteField("offset_ms", "0")
	fw, err := mw.CreateFormFile("audio", "chunk.wav")
	if err != nil {
		return nil, err
	}
	_, _ = fw.Write(audioData)
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/chunk", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req, nil
}

func TestSessionEndpoints(t *testing.T) {
	s := store.New()
	h := &Handler{Store: s}

	// 1. Create Session
	req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	rr := httptest.NewRecorder()
	h.CreateSession(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rr.Code)
	}

	var sessResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &sessResp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if sessResp.SessionID == "" {
		t.Fatalf("expected non-empty session_id")
	}

	// 2. Debug Session
	reqDebug := httptest.NewRequest(http.MethodGet, "/api/debug/session/"+sessResp.SessionID, nil)
	reqDebug.SetPathValue("id", sessResp.SessionID)
	rrDebug := httptest.NewRecorder()
	h.DebugSession(rrDebug, reqDebug)

	if rrDebug.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rrDebug.Code)
	}
}

// Acceptance Test 2: Quote verification (if quote is not in transcript, discard interrupt).
func TestQuoteVerification_FakeProviderDiscarded(t *testing.T) {
	whisperMock := setupMockWhisperServer("We are working on a new presentation rehearsing tool.")
	defer whisperMock.Close()

	s := store.New()
	sessID := "test-session-quote-verify"
	s.Create(sessID)

	os.Setenv("MODEL_PROVIDER", "fake")
	fakeLLM, err := llm.New()
	if err != nil {
		t.Fatalf("failed to create fake provider: %v", err)
	}

	h := &Handler{
		Store: s,
		ASR:   asr.New(whisperMock.URL),
		LLM:   fakeLLM,
	}

	req, err := createMultipartChunkRequest(sessID, "speech", 0, []byte("fake audio bytes"))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	rr := httptest.NewRecorder()
	h.ReceiveChunk(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp chunkResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.Interrupt != nil {
		t.Fatalf("expected interrupt=null because fake quote is NOT in transcript, but got: %+v", resp.Interrupt)
	}
	t.Log("PASS: fake provider quote was successfully discarded, interrupt: null")
}

// Acceptance Test 3: Cooldown (60s cooldown suppresses second interrupt).
func TestCooldown_60sSuppression(t *testing.T) {
	transcript := "Our claims are verified word for word."
	whisperMock := setupMockWhisperServer(transcript)
	defer whisperMock.Close()

	s := store.New()
	sessID := "test-session-cooldown"
	s.Create(sessID)

	os.Setenv("MODEL_PROVIDER", "fake_valid")
	fakeValidLLM, err := llm.New()
	if err != nil {
		t.Fatalf("failed to create fake_valid provider: %v", err)
	}

	h := &Handler{
		Store: s,
		ASR:   asr.New(whisperMock.URL),
		LLM:   fakeValidLLM,
	}

	// Chunk 1: should fire interrupt
	req1, _ := createMultipartChunkRequest(sessID, "speech", 0, []byte("chunk 1 audio"))
	rr1 := httptest.NewRecorder()
	h.ReceiveChunk(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("chunk 1 failed: %d: %s", rr1.Code, rr1.Body.String())
	}

	var resp1 chunkResponse
	if err := json.Unmarshal(rr1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode chunk 1: %v", err)
	}
	if resp1.Interrupt == nil {
		t.Fatalf("expected Chunk 1 to fire an interrupt, got null")
	}
	t.Logf("PASS: Chunk 1 interrupt fired: %+v", resp1.Interrupt)

	// Chunk 2 (immediate): should be suppressed by 60s cooldown
	req2, _ := createMultipartChunkRequest(sessID, "speech", 3000, []byte("chunk 2 audio"))
	rr2 := httptest.NewRecorder()
	h.ReceiveChunk(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("chunk 2 failed: %d: %s", rr2.Code, rr2.Body.String())
	}

	var resp2 chunkResponse
	if err := json.Unmarshal(rr2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode chunk 2: %v", err)
	}
	if resp2.Interrupt != nil {
		t.Fatalf("expected Chunk 2 interrupt to be suppressed by 60s cooldown, but got: %+v", resp2.Interrupt)
	}
	t.Log("PASS: Chunk 2 interrupt suppressed by cooldown: null")

	// Simulate passage of 61 seconds
	past := time.Now().Add(-61 * time.Second)
	h.lastInterrupt.Store(sessID, past)

	// Chunk 3: cooldown has expired, should fire again
	req3, _ := createMultipartChunkRequest(sessID, "speech", 65000, []byte("chunk 3 audio"))
	rr3 := httptest.NewRecorder()
	h.ReceiveChunk(rr3, req3)

	var resp3 chunkResponse
	if err := json.Unmarshal(rr3.Body.Bytes(), &resp3); err != nil {
		t.Fatalf("decode chunk 3: %v", err)
	}
	if resp3.Interrupt == nil {
		t.Fatalf("expected Chunk 3 to fire after cooldown elapsed, got null")
	}
	t.Logf("PASS: Chunk 3 fired after cooldown: %+v", resp3.Interrupt)
}

// Acceptance Test 1: Full pipeline with Groq on unsupported claim.
func TestGroqInterruption_LivePipeline(t *testing.T) {
	_ = godotenv.Load("../../.env")
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		t.Skip("skipping live Groq pipeline test: GROQ_API_KEY not set")
	}

	claim := "our approach is 10 times faster than anything else on the market."
	whisperMock := setupMockWhisperServer(claim)
	defer whisperMock.Close()

	s := store.New()
	sessID := "test-session-groq-live"
	s.Create(sessID)

	os.Setenv("MODEL_PROVIDER", "groq")
	groqLLM, err := llm.New()
	if err != nil {
		t.Fatalf("failed to create Groq LLM: %v", err)
	}

	h := &Handler{
		Store:         s,
		ASR:           asr.New(whisperMock.URL),
		LLM:           groqLLM,
		lastInterrupt: sync.Map{},
	}

	req, err := createMultipartChunkRequest(sessID, "speech", 0, []byte("audio claim"))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	rr := httptest.NewRecorder()
	h.ReceiveChunk(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("chunk request failed: %d: %s", rr.Code, rr.Body.String())
	}

	var resp chunkResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.Interrupt == nil {
		t.Fatalf("expected Groq to trigger interrupt on claim %q, got null", claim)
	}

	if resp.Interrupt.Trigger != "unsupported_claim" {
		t.Logf("Warning: trigger is %s (expected unsupported_claim)", resp.Interrupt.Trigger)
	}

	t.Logf("PASS: Groq fired interrupt on claim: trigger=%s quote=%q question=%q audio_url=%s",
		resp.Interrupt.Trigger, resp.Interrupt.Quote, resp.Interrupt.Question, resp.Interrupt.AudioURL)
}

func TestFinishAndHistoryEndpoints(t *testing.T) {
	s := store.New()
	sessID := "test-finish-sess"
	sess := s.Create(sessID)
	_ = s.AppendChunk(sessID, store.Chunk{
		Kind:     "speech",
		OffsetMs: 0,
		Text:     "Good afternoon everyone. We are excited to present our architecture.",
		Words: []store.Word{
			{Word: "Good", StartMs: 0, EndMs: 400},
			{Word: "afternoon", StartMs: 500, EndMs: 1000},
			{Word: "everyone", StartMs: 1100, EndMs: 1600},
			{Word: "We", StartMs: 1700, EndMs: 1900},
			{Word: "are", StartMs: 2000, EndMs: 2200},
			{Word: "excited", StartMs: 2300, EndMs: 2700},
			{Word: "to", StartMs: 2800, EndMs: 2900},
			{Word: "present", StartMs: 3000, EndMs: 3400},
			{Word: "our", StartMs: 3500, EndMs: 3700},
			{Word: "architecture", StartMs: 3800, EndMs: 4400},
		},
	})
	_ = sess

	h := &Handler{
		Store: s,
	}

	// 1. Test POST /api/finish
	finishPayload := []byte(`{"session_id": "test-finish-sess", "duration_seconds": 120}`)
	reqFinish := httptest.NewRequest(http.MethodPost, "/api/finish", bytes.NewReader(finishPayload))
	reqFinish.Header.Set("Content-Type", "application/json")
	rrFinish := httptest.NewRecorder()
	h.FinishSession(rrFinish, reqFinish)

	if rrFinish.Code != http.StatusOK {
		t.Fatalf("finish session failed: %d: %s", rrFinish.Code, rrFinish.Body.String())
	}

	var finishResp map[string]any
	if err := json.Unmarshal(rrFinish.Body.Bytes(), &finishResp); err != nil {
		t.Fatalf("decode finish response: %v", err)
	}

	if finishResp["session_id"] != sessID {
		t.Errorf("expected session_id %s, got %v", sessID, finishResp["session_id"])
	}
	if finishResp["overall"] == nil {
		t.Errorf("expected overall score in response")
	}

	// 2. Test GET /history
	reqHist := httptest.NewRequest(http.MethodGet, "/history", nil)
	rrHist := httptest.NewRecorder()
	h.GetHistory(rrHist, reqHist)

	if rrHist.Code != http.StatusOK {
		t.Fatalf("get history failed: %d: %s", rrHist.Code, rrHist.Body.String())
	}

	var historyList []map[string]any
	if err := json.Unmarshal(rrHist.Body.Bytes(), &historyList); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if len(historyList) == 0 {
		t.Fatalf("expected non-empty history list")
	}
	// The finished session should be prepended as first item
	if historyList[0]["session_id"] != sessID {
		t.Errorf("expected newest session %s at index 0, got %v", sessID, historyList[0]["session_id"])
	}

	t.Logf("PASS: FinishSession and GetHistory verified successfully (history count=%d)", len(historyList))
}

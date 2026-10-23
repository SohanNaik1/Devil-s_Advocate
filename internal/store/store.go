// Package store provides a thread-safe in-memory session store.
package store

import (
	"fmt"
	"sync"
	"time"
)

// Word is a single transcribed token with millisecond timestamps.
type Word struct {
	Word    string `json:"word"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

// Chunk is one audio segment received via POST /api/chunk.
type Chunk struct {
	Kind     string `json:"kind"` // "speech" | "answer"
	OffsetMs int    `json:"offset_ms"`
	Text     string `json:"text"`
	Words    []Word `json:"words"`
}

// InterruptionRecord tracks an AI challenge during the session.
type InterruptionRecord struct {
	ID             string `json:"id"`
	AtMs           int    `json:"at_ms"`
	Timestamp      string `json:"timestamp"`
	Trigger        string `json:"trigger"`
	Quote          string `json:"quote"`
	Question       string `json:"question"`
	AudioURL       string `json:"audio_url"`
	AnswerText     string `json:"answer_text"`
	UserAnswer     string `json:"user_answer"`
	AnswerRating   int    `json:"answer_rating"`
	ResponseTimeMs int    `json:"response_time_ms"`
	Resolved       bool   `json:"resolved"`
}

// Session holds the growing state for one rehearsal session.
type Session struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	CreatedAt     time.Time            `json:"created_at"`
	Chunks        []Chunk              `json:"chunks"`
	Interruptions []InterruptionRecord `json:"interruptions"`
}

// FullText returns the concatenated transcript text across all chunks.
func (s *Session) FullText() string {
	out := ""
	for _, c := range s.Chunks {
		if out != "" {
			out += " "
		}
		out += c.Text
	}
	return out
}

// SpeechText returns only spoken chunks (excluding answers).
func (s *Session) SpeechText() string {
	out := ""
	for _, c := range s.Chunks {
		if c.Kind == "answer" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += c.Text
	}
	return out
}

// AllWords returns all words across all chunks, with offset_ms applied.
func (s *Session) AllWords() []Word {
	var all []Word
	for _, c := range s.Chunks {
		for _, w := range c.Words {
			all = append(all, Word{
				Word:    w.Word,
				StartMs: w.StartMs + c.OffsetMs,
				EndMs:   w.EndMs + c.OffsetMs,
			})
		}
	}
	return all
}

// HistoryEntry represents a concise session overview for GET /history.
type HistoryEntry struct {
	ID                 string         `json:"id"`
	SessionID          string         `json:"session_id"`
	Timestamp          string         `json:"timestamp"`
	CreatedAt          string         `json:"created_at"`
	Title              string         `json:"title"`
	Overall            int            `json:"overall"`
	Delivery           int            `json:"delivery"`
	Content            int            `json:"content"`
	Scores             map[string]int `json:"scores"`
	Duration           string         `json:"duration"`
	DurationSeconds    int            `json:"duration_seconds"`
	InterruptionsCount int            `json:"interruptions_count"`
	WPMAvg             int            `json:"wpm_avg"`
}

// Store is the in-memory session registry and report store.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	reports  map[string]any
	history  []HistoryEntry
}

// New returns a ready-to-use Store pre-seeded with baseline sample history.
func New() *Store {
	initialHistory := []HistoryEntry{
		{
			ID:              "sess_1001",
			SessionID:       "sess_1001",
			Timestamp:       "2026-10-23 13:45",
			CreatedAt:       "2026-10-23T13:45:00Z",
			Title:           "Self-Rewriting AI Architecture Pitch",
			Overall:         84,
			Delivery:        79,
			Content:         88,
			Scores:          map[string]int{"overall": 84, "delivery": 79, "content": 88},
			Duration:        "03:12",
			DurationSeconds: 192,
			InterruptionsCount: 2,
			WPMAvg:          133,
		},
		{
			ID:              "sess_1000",
			SessionID:       "sess_1000",
			Timestamp:       "2026-10-23 11:20",
			CreatedAt:       "2026-10-23T11:20:00Z",
			Title:           "Autonomous Agent Infrastructure",
			Overall:         76,
			Delivery:        82,
			Content:         71,
			Scores:          map[string]int{"overall": 76, "delivery": 82, "content": 71},
			Duration:        "02:40",
			DurationSeconds: 160,
			InterruptionsCount: 3,
			WPMAvg:          146,
		},
		{
			ID:              "sess_999",
			SessionID:       "sess_999",
			Timestamp:       "2026-10-22 17:05",
			CreatedAt:       "2026-10-22T17:05:00Z",
			Title:           "Decentralized Model Consensus Protocol",
			Overall:         91,
			Delivery:        89,
			Content:         93,
			Scores:          map[string]int{"overall": 91, "delivery": 89, "content": 93},
			Duration:        "04:15",
			DurationSeconds: 255,
			InterruptionsCount: 1,
			WPMAvg:          128,
		},
		{
			ID:              "sess_998",
			SessionID:       "sess_998",
			Timestamp:       "2026-10-22 14:18",
			CreatedAt:       "2026-10-22T14:18:00Z",
			Title:           "Zero-Knowledge Rollup Verification",
			Overall:         80,
			Delivery:        75,
			Content:         85,
			Scores:          map[string]int{"overall": 80, "delivery": 75, "content": 85},
			Duration:        "03:00",
			DurationSeconds: 180,
			InterruptionsCount: 2,
			WPMAvg:          139,
		},
		{
			ID:              "sess_997",
			SessionID:       "sess_997",
			Timestamp:       "2026-10-21 16:50",
			CreatedAt:       "2026-10-21T16:50:00Z",
			Title:           "Dynamic Kernel Compaction Engine",
			Overall:         87,
			Delivery:        85,
			Content:         89,
			Scores:          map[string]int{"overall": 87, "delivery": 85, "content": 89},
			Duration:        "03:35",
			DurationSeconds: 215,
			InterruptionsCount: 2,
			WPMAvg:          135,
		},
	}

	return &Store{
		sessions: make(map[string]*Session),
		reports:  make(map[string]any),
		history:  initialHistory,
	}
}

// Create inserts a new session and returns it.
func (s *Store) Create(id string) *Session {
	sess := &Session{
		ID:        id,
		Title:     "Presentation Rehearsal",
		CreatedAt: time.Now(),
		Chunks:    make([]Chunk, 0),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

// Get retrieves a session by ID. Returns an error if not found.
func (s *Store) Get(id string) (*Session, error) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return sess, nil
}

// AppendChunk appends a transcribed chunk to an existing session.
func (s *Store) AppendChunk(id string, chunk Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	sess.Chunks = append(sess.Chunks, chunk)

	// If kind is "answer" and an interruption exists, record it as user_answer
	if chunk.Kind == "answer" && len(sess.Interruptions) > 0 {
		lastIdx := len(sess.Interruptions) - 1
		if sess.Interruptions[lastIdx].AnswerText != "" {
			sess.Interruptions[lastIdx].AnswerText += " " + chunk.Text
		} else {
			sess.Interruptions[lastIdx].AnswerText = chunk.Text
		}
		sess.Interruptions[lastIdx].UserAnswer = sess.Interruptions[lastIdx].AnswerText
		sess.Interruptions[lastIdx].Resolved = true
	}

	return nil
}

// RecordInterruption records an AI challenge against the session.
func (s *Store) RecordInterruption(id string, rec InterruptionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	sess.Interruptions = append(sess.Interruptions, rec)
	return nil
}

// SaveReport stores the generated scorecard and prepends to the history list.
func (s *Store) SaveReport(sessionID string, report any, entry HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[sessionID] = report
	// Prepend new history entry, keep max 10
	s.history = append([]HistoryEntry{entry}, s.history...)
	if len(s.history) > 10 {
		s.history = s.history[:10]
	}
}

// GetReport retrieves a saved report by session ID.
func (s *Store) GetReport(sessionID string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rep, ok := s.reports[sessionID]
	if !ok {
		return nil, fmt.Errorf("report for session %q not found", sessionID)
	}
	return rep, nil
}

// ListHistory returns the last 10 session overviews.
func (s *Store) ListHistory() []HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]HistoryEntry, len(s.history))
	copy(cp, s.history)
	return cp
}

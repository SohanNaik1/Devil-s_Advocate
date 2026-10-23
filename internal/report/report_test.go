package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/SohanNaik1/Devils-Advocate/internal/store"
)

type dummyProvider struct{}

func (d *dummyProvider) Decide(_ string) (*struct {
	Interrupt bool
	Trigger   string
	Quote     string
	Question  string
}, error) {
	return nil, nil
}

func (d *dummyProvider) Evaluate(_ string) (string, error) {
	return `{
		"score": 88,
		"structure": 90,
		"clarity": 86,
		"evidence": 85,
		"answers": 92,
		"went_well": [
			{"point": "Clear problem statement", "quote": "we guarantee high reliability"}
		],
		"to_fix": [
			{"point": "Clarify latency impact", "quote": "very fast execution", "suggestion": "Add millisecond benchmarks"}
		]
	}`, nil
}

func TestGenerateReport(t *testing.T) {
	sess := &store.Session{
		ID:        "test-session-123",
		Title:     "Decentralized Consensus Engine",
		CreatedAt: time.Now(),
		Chunks: []store.Chunk{
			{
				Kind:     "speech",
				OffsetMs: 0,
				Text:     "Good morning everyone today we present our new system we guarantee high reliability and very fast execution",
				Words: []store.Word{
					{Word: "Good", StartMs: 0, EndMs: 400},
					{Word: "morning", StartMs: 500, EndMs: 900},
					{Word: "everyone", StartMs: 1000, EndMs: 1400},
					{Word: "today", StartMs: 1500, EndMs: 1900},
					{Word: "we", StartMs: 2000, EndMs: 2300},
					{Word: "present", StartMs: 2400, EndMs: 2800},
					{Word: "our", StartMs: 2900, EndMs: 3100},
					{Word: "new", StartMs: 3200, EndMs: 3500},
					{Word: "system", StartMs: 3600, EndMs: 4000},
					{Word: "we", StartMs: 6500, EndMs: 6800}, // >2000ms pause here (4000 to 6500)
					{Word: "guarantee", StartMs: 6900, EndMs: 7400},
					{Word: "high", StartMs: 7500, EndMs: 7800},
					{Word: "reliability", StartMs: 7900, EndMs: 8500},
					{Word: "and", StartMs: 8600, EndMs: 8800},
					{Word: "very", StartMs: 8900, EndMs: 9200},
					{Word: "fast", StartMs: 9300, EndMs: 9600},
					{Word: "execution", StartMs: 9700, EndMs: 10200},
				},
			},
		},
		Interruptions: []store.InterruptionRecord{
			{
				ID:         "int-1",
				AtMs:       4000,
				Trigger:    "unsupported_claim",
				Quote:      "we guarantee high reliability",
				Question:   "What failover tests prove this?",
				AnswerText: "We tested Byzantine node failures across 5 clusters.",
				UserAnswer: "We tested Byzantine node failures across 5 clusters.",
				Resolved:   true,
			},
		},
	}

	rep, err := Generate(sess, nil, 120)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rep.SessionID != "test-session-123" {
		t.Errorf("expected session_id test-session-123, got %s", rep.SessionID)
	}
	if rep.Overall <= 0 || rep.Overall > 100 {
		t.Errorf("invalid overall score: %d", rep.Overall)
	}
	if len(rep.LongPauses) != 1 {
		t.Errorf("expected 1 long pause, got %d", len(rep.LongPauses))
	}
	if len(rep.Interruptions) != 1 {
		t.Errorf("expected 1 interruption, got %d", len(rep.Interruptions))
	}

	// Verify JSON marshaling works cleanly
	bytes, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("failed to marshal report JSON: %v", err)
	}
	if len(bytes) == 0 {
		t.Fatal("empty json output")
	}

	var parsed map[string]any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated json: %v", err)
	}

	// Verify required keys
	for _, key := range []string{"session_id", "overall", "delivery", "content", "interruptions", "wpm_by_30s", "went_well", "to_fix"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing expected JSON key: %s", key)
		}
	}
}

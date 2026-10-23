// Package metrics computes delivery analytics from Whisper word-level timestamps.
//
// Input: the session's full slice of Chunk records (each containing Words with
// millisecond-precision start/end times and the kind "speech" | "answer").
//
// Output: a Delivery struct that maps directly to the delivery JSON shape:
//
//	{
//	  "score": 0,
//	  "wpm": 0,
//	  "wpm_by_30s": [],
//	  "filler_count": 0,
//	  "fillers": {},
//	  "fillers_per_min": 0,
//	  "long_pauses": [{"start_ms": 0, "duration_ms": 0}],
//	  "longest_pause_ms": 0,
//	  "talk_ratio": 0
//	}
//
// Scoring rules:
//
//   - Start at 100.
//   - Pace penalty: 0 if 110 ≤ WPM ≤ 160. Otherwise max(0, dist-10)*2, capped at 30.
//   - Filler penalty: fillers_per_min * 5, capped at 30.
//   - Pause penalty: len(long_pauses) * 8, capped at 30.
//   - Floor: 0.
package metrics

import (
	"math"
	"strings"

	"github.com/SohanNaik1/Devils-Advocate/internal/store"
)

// ---------------------------------------------------------------------------
// Filler word list
// ---------------------------------------------------------------------------

// fillerSet is the canonical list of filler words/phrases.
// Multi-word fillers are matched by joining consecutive words.
var fillerPhrases = []string{
	"you know",
	"sort of",
	"kind of",
}

var fillerWords = map[string]bool{
	"um":        true,
	"uh":        true,
	"er":        true,
	"ah":        true,
	"like":      true,
	"basically": true,
	"actually":  true,
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------

// Pause represents a silence gap >= 2000 ms between consecutive spoken words.
type Pause struct {
	StartMs    int `json:"start_ms"`
	DurationMs int `json:"duration_ms"`
}

// Delivery is the full delivery analytics payload.
type Delivery struct {
	Score          int            `json:"score"`
	WPM            float64        `json:"wpm"`
	WPMBy30s       []float64      `json:"wpm_by_30s"`
	FillerCount    int            `json:"filler_count"`
	Fillers        map[string]int `json:"fillers"`
	FillersPerMin  float64        `json:"fillers_per_min"`
	LongPauses     []Pause        `json:"long_pauses"`
	LongestPauseMs int            `json:"longest_pause_ms"`
	TalkRatio      float64        `json:"talk_ratio"`
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	longPauseThresholdMs = 2000 // gap between words that counts as a long pause
	wpmWindowMs          = 30_000
	wpmLow               = 110.0
	wpmHigh              = 160.0
	maxPenalty           = 30.0
)

// ---------------------------------------------------------------------------
// Compute
// ---------------------------------------------------------------------------

// Compute calculates delivery analytics from a session's chunks.
// Only "speech" chunks contribute to WPM/filler/pause; "answer" chunks are
// included in the word stream but treated as speech (they use the same
// timestamps from Whisper).
func Compute(chunks []store.Chunk) Delivery {
	// Flatten words from all chunks into a single global timeline using offset_ms.
	type globalWord struct {
		text    string
		startMs int
		endMs   int
	}

	var allWords []globalWord
	for _, c := range chunks {
		for _, w := range c.Words {
			allWords = append(allWords, globalWord{
				text:    strings.ToLower(strings.TrimSpace(w.Word)),
				startMs: w.StartMs + c.OffsetMs,
				endMs:   w.EndMs + c.OffsetMs,
			})
		}
	}

	if len(allWords) == 0 {
		return Delivery{
			Score:    100,
			Fillers:  map[string]int{},
			WPMBy30s: []float64{},
			LongPauses: []Pause{},
		}
	}

	// ── Session time boundaries ─────────────────────────────────────────────
	firstMs := allWords[0].startMs
	lastMs  := allWords[len(allWords)-1].endMs
	totalSessionMs := lastMs - firstMs
	if totalSessionMs <= 0 {
		totalSessionMs = 1
	}

	// ── Talk time: sum of all word durations ─────────────────────────────────
	talkMs := 0
	for _, w := range allWords {
		d := w.endMs - w.startMs
		if d > 0 {
			talkMs += d
		}
	}
	talkRatio := math.Round(float64(talkMs)/float64(totalSessionMs)*100) / 100

	// ── Overall WPM ─────────────────────────────────────────────────────────
	durationMin := float64(totalSessionMs) / 60_000.0
	overallWPM := 0.0
	if durationMin > 0 {
		overallWPM = math.Round(float64(len(allWords))/durationMin*10) / 10
	}

	// ── WPM by 30-second windows ─────────────────────────────────────────────
	var wpmBy30s []float64
	windowStart := firstMs
	for windowStart < lastMs {
		windowEnd := windowStart + wpmWindowMs
		count := 0
		for _, w := range allWords {
			if w.startMs >= windowStart && w.startMs < windowEnd {
				count++
			}
		}
		windowDurationMin := math.Min(float64(windowEnd-windowStart), float64(lastMs-windowStart)) / 60_000.0
		if windowDurationMin > 0 {
			wpmBy30s = append(wpmBy30s, math.Round(float64(count)/windowDurationMin*10)/10)
		}
		windowStart = windowEnd
	}
	if len(wpmBy30s) == 0 {
		wpmBy30s = []float64{}
	}

	// ── Filler words ─────────────────────────────────────────────────────────
	fillers := map[string]int{}

	// Build a sequence of cleaned tokens for multi-word phrase matching.
	tokens := make([]string, len(allWords))
	for i, w := range allWords {
		tokens[i] = w.text
	}

	// Mark matched indices to avoid double-counting.
	matched := make([]bool, len(tokens))

	// 1. Multi-word phrases first (greedy, left-to-right).
	for _, phrase := range fillerPhrases {
		parts := strings.Fields(phrase)
		n := len(parts)
		for i := 0; i <= len(tokens)-n; i++ {
			if matched[i] {
				continue
			}
			ok := true
			for j := 0; j < n; j++ {
				if matched[i+j] || tokens[i+j] != parts[j] {
					ok = false
					break
				}
			}
			if ok {
				fillers[phrase]++
				for j := 0; j < n; j++ {
					matched[i+j] = true
				}
			}
		}
	}

	// 2. Single-word fillers.
	for i, tok := range tokens {
		if matched[i] {
			continue
		}
		// Strip leading/trailing punctuation.
		clean := strings.Trim(tok, ".,!?;:\"'")
		if fillerWords[clean] {
			fillers[clean]++
			matched[i] = true
		}
	}

	fillerCount := 0
	for _, c := range fillers {
		fillerCount += c
	}

	fillersPerMin := 0.0
	if durationMin > 0 {
		fillersPerMin = math.Round(float64(fillerCount)/durationMin*100) / 100
	}

	// ── Long pauses ──────────────────────────────────────────────────────────
	var longPauses []Pause
	longestPauseMs := 0

	for i := 1; i < len(allWords); i++ {
		gap := allWords[i].startMs - allWords[i-1].endMs
		if gap >= longPauseThresholdMs {
			p := Pause{
				StartMs:    allWords[i-1].endMs,
				DurationMs: gap,
			}
			longPauses = append(longPauses, p)
			if gap > longestPauseMs {
				longestPauseMs = gap
			}
		}
	}
	if longPauses == nil {
		longPauses = []Pause{}
	}

	// ── Score ────────────────────────────────────────────────────────────────
	score := 100.0

	// Pace penalty.
	pacePenalty := 0.0
	if overallWPM < wpmLow {
		dist := wpmLow - overallWPM
		if dist > 10 {
			pacePenalty = (dist - 10) * 2
		}
	} else if overallWPM > wpmHigh {
		dist := overallWPM - wpmHigh
		if dist > 10 {
			pacePenalty = (dist - 10) * 2
		}
	}
	pacePenalty = math.Min(pacePenalty, maxPenalty)

	// Filler penalty.
	fillerPenalty := math.Min(fillersPerMin*5, maxPenalty)

	// Pause penalty.
	pausePenalty := math.Min(float64(len(longPauses))*8, maxPenalty)

	score -= pacePenalty + fillerPenalty + pausePenalty
	if score < 0 {
		score = 0
	}

	return Delivery{
		Score:          int(math.Round(score)),
		WPM:            overallWPM,
		WPMBy30s:       wpmBy30s,
		FillerCount:    fillerCount,
		Fillers:        fillers,
		FillersPerMin:  fillersPerMin,
		LongPauses:     longPauses,
		LongestPauseMs: longestPauseMs,
		TalkRatio:      talkRatio,
	}
}

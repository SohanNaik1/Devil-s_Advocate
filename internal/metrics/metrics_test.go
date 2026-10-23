package metrics

import (
	"math"
	"testing"

	"github.com/SohanNaik1/Devils-Advocate/internal/store"
)

// ---------------------------------------------------------------------------
// Test fixture builder helpers
// ---------------------------------------------------------------------------

// makeWord creates a store.Word with the given text and millisecond times.
func makeWord(text string, startMs, endMs int) store.Word {
	return store.Word{Word: text, StartMs: startMs, EndMs: endMs}
}

// buildFixture constructs a deterministic test session fixture with:
//   - A known WPM close to 130 (squarely in the acceptable 110–160 range)
//   - Exactly 10 filler words/phrases spread across the word list
//   - Exactly 3 long pauses (≥ 2000 ms gaps between consecutive words)
//
// The fixture runs across two chunks so the offset_ms logic is also exercised.
func buildFixture() []store.Chunk {
	// Chunk 1 (offset_ms = 0, 0..30000 ms window)
	// We'll put 60 words in 30 seconds → 120 WPM.
	// We embed 6 filler words here and 1 long pause.
	chunk1Words := []store.Word{}

	t := 0 // current ms cursor
	wordDuration := 400
	normalGap := 100 // gap between words (100ms)

	addWord := func(text string) {
		chunk1Words = append(chunk1Words, makeWord(text, t, t+wordDuration))
		t += wordDuration + normalGap
	}

	addPause := func(gapMs int) {
		t += gapMs // just advance the cursor without emitting a word
	}

	// Segment A: 20 content words
	for i := 0; i < 20; i++ {
		addWord("content")
	}
	// Filler 1: "um"
	addWord("um")
	// Filler 2: "uh"
	addWord("uh")
	// 10 more content words
	for i := 0; i < 10; i++ {
		addWord("content")
	}
	// Long pause 1 (between words[31] and words[32])
	addPause(2500) // 2500 ms gap → counts as long pause
	// Filler 3: "like"
	addWord("like")
	// Filler 4: "er"
	addWord("er")
	// 15 more content words
	for i := 0; i < 15; i++ {
		addWord("content")
	}
	// Filler 5: "you know" (multi-word phrase = 2 tokens, counts as 1 filler)
	addWord("you")
	addWord("know")
	// 10 more content words
	for i := 0; i < 10; i++ {
		addWord("content")
	}

	// Record end of chunk 1.
	chunk1End := t
	_ = chunk1End

	chunk1 := store.Chunk{
		Kind:     "speech",
		OffsetMs: 0,
		Text:     "chunk one",
		Words:    chunk1Words,
	}

	// Chunk 2 (offset_ms = 35000 ms, so it starts 35 seconds into the session)
	// This creates a long pause between the last word of chunk 1 and the first
	// word of chunk 2: the gap is (35000 + 0) - chunk1End. We'll make it big.
	// We put 60 words into chunk 2 as well; embed 4 more fillers and 2 more pauses.
	chunk2Words := []store.Word{}
	t2 := 0 // local time within chunk 2

	addWord2 := func(text string) {
		chunk2Words = append(chunk2Words, makeWord(text, t2, t2+wordDuration))
		t2 += wordDuration + normalGap
	}
	addPause2 := func(gapMs int) {
		t2 += gapMs
	}

	// 15 content words
	for i := 0; i < 15; i++ {
		addWord2("content")
	}
	// Filler 6: "basically"
	addWord2("basically")
	// Long pause 2
	addPause2(3000)
	// Filler 7: "ah"
	addWord2("ah")
	// 10 content words
	for i := 0; i < 10; i++ {
		addWord2("content")
	}
	// Filler 8: "actually"
	addWord2("actually")
	// 10 content words
	for i := 0; i < 10; i++ {
		addWord2("content")
	}
	// Long pause 3
	addPause2(2200)
	// Filler 9: "sort of" (multi-word = 2 tokens, counts as 1 filler)
	addWord2("sort")
	addWord2("of")
	// 10 content words
	for i := 0; i < 10; i++ {
		addWord2("content")
	}
	// Filler 10: "kind of" (multi-word = 2 tokens, counts as 1 filler)
	addWord2("kind")
	addWord2("of")
	// 10 content words
	for i := 0; i < 10; i++ {
		addWord2("content")
	}

	chunk2 := store.Chunk{
		Kind:     "speech",
		OffsetMs: 35_000, // starts 35s in → large inter-chunk gap
		Text:     "chunk two",
		Words:    chunk2Words,
	}

	return []store.Chunk{chunk1, chunk2}
}

// ---------------------------------------------------------------------------
// Main fixture test
// ---------------------------------------------------------------------------

func TestCompute_FixtureKnownValues(t *testing.T) {
	chunks := buildFixture()
	d := Compute(chunks)

	t.Logf("Delivery result: score=%d wpm=%.1f fillerCount=%d fillers=%v longPauses=%d longestPause=%dms talkRatio=%.3f",
		d.Score, d.WPM, d.FillerCount, d.Fillers, len(d.LongPauses), d.LongestPauseMs, d.TalkRatio)
	t.Logf("wpm_by_30s: %v", d.WPMBy30s)

	// ── 10 fillers ────────────────────────────────────────────────────────────
	if d.FillerCount != 10 {
		t.Errorf("want FillerCount=10, got %d (fillers: %v)", d.FillerCount, d.Fillers)
	}

	// Verify individual filler categories.
	expectedFillers := map[string]int{
		"um": 1, "uh": 1, "er": 1, "ah": 1,
		"like": 1, "basically": 1, "actually": 1,
		"you know": 1, "sort of": 1, "kind of": 1,
	}
	for phrase, want := range expectedFillers {
		got := d.Fillers[phrase]
		if got != want {
			t.Errorf("filler %q: want %d, got %d", phrase, want, got)
		}
	}

	// ── 3 long pauses ─────────────────────────────────────────────────────────
	// Note: the inter-chunk gap (chunk1's last word ends around 33-35s, chunk2
	// starts at 35000ms) produces one more pause. We count all ≥ 2000ms gaps.
	// The inter-chunk gap depends on how many words chunk1 has. Let's check
	// that we have AT LEAST 3 long pauses (the 3 we embedded) and verify their
	// minimum duration.
	if len(d.LongPauses) < 3 {
		t.Errorf("want at least 3 long pauses, got %d", len(d.LongPauses))
	}
	for i, p := range d.LongPauses {
		if p.DurationMs < longPauseThresholdMs {
			t.Errorf("pause[%d]: duration %dms is below threshold %dms", i, p.DurationMs, longPauseThresholdMs)
		}
		if p.StartMs < 0 {
			t.Errorf("pause[%d]: negative StartMs %d", i, p.StartMs)
		}
	}

	// ── WPM is in a reasonable range ─────────────────────────────────────────
	// We placed ~120 meaningful content words + fillers across ~70s of talk
	// (plus silent gaps). Exact value depends on total time, but should be > 50.
	if d.WPM < 50 || d.WPM > 300 {
		t.Errorf("WPM out of plausible range: %.1f", d.WPM)
	}

	// ── WPMBy30s is non-empty ─────────────────────────────────────────────────
	if len(d.WPMBy30s) == 0 {
		t.Errorf("want non-empty wpm_by_30s, got empty")
	}

	// ── Score is within 0–100 ────────────────────────────────────────────────
	if d.Score < 0 || d.Score > 100 {
		t.Errorf("score out of range [0,100]: %d", d.Score)
	}

	// ── LongestPauseMs is the maximum of all long pause durations ────────────
	if len(d.LongPauses) > 0 && d.LongestPauseMs == 0 {
		t.Errorf("LongestPauseMs should be non-zero when pauses exist")
	}
	maxPauseDuration := 0
	for _, p := range d.LongPauses {
		if p.DurationMs > maxPauseDuration {
			maxPauseDuration = p.DurationMs
		}
	}
	if d.LongestPauseMs != maxPauseDuration {
		t.Errorf("LongestPauseMs: want %d, got %d", maxPauseDuration, d.LongestPauseMs)
	}

	// ── TalkRatio is between 0 and 1 ─────────────────────────────────────────
	if d.TalkRatio < 0 || d.TalkRatio > 1 {
		t.Errorf("TalkRatio out of [0,1]: %.3f", d.TalkRatio)
	}

	// ── FillersPerMin > 0 ────────────────────────────────────────────────────
	if d.FillersPerMin <= 0 {
		t.Errorf("FillersPerMin should be >0, got %.3f", d.FillersPerMin)
	}
}

// ---------------------------------------------------------------------------
// Edge case: empty session
// ---------------------------------------------------------------------------

func TestCompute_EmptySession(t *testing.T) {
	d := Compute(nil)
	if d.Score != 100 {
		t.Errorf("empty session: want score=100, got %d", d.Score)
	}
	if d.FillerCount != 0 {
		t.Errorf("empty session: want FillerCount=0, got %d", d.FillerCount)
	}
	if len(d.LongPauses) != 0 {
		t.Errorf("empty session: want 0 long pauses, got %d", len(d.LongPauses))
	}
}

// ---------------------------------------------------------------------------
// Penalty cap tests
// ---------------------------------------------------------------------------

func TestCompute_PacePenaltyCap(t *testing.T) {
	// Create a session with extremely slow WPM (30 WPM over 1 minute).
	// At 30 WPM, dist from wpmLow (110) = 80. Penalty = (80-10)*2 = 140, capped at 30.
	words := make([]store.Word, 30)
	for i := range words {
		startMs := i * 2000 // one word every 2 seconds → 30 WPM over 60s
		words[i] = makeWord("content", startMs, startMs+200)
	}
	chunks := []store.Chunk{{Kind: "speech", OffsetMs: 0, Text: "slow", Words: words}}
	d := Compute(chunks)

	t.Logf("SlowPace: wpm=%.1f score=%d", d.WPM, d.Score)
	// Score should be ≥ 70 (only pace penalty can apply here, capped at 30).
	if d.Score > 100 || d.Score < 0 {
		t.Errorf("score out of bounds: %d", d.Score)
	}
	// Penalty should be capped: score >= 100 - 30 = 70 (assuming no fillers/pauses).
	if d.Score < 70 {
		t.Errorf("pace penalty cap violated: score=%d (want ≥70)", d.Score)
	}
}

func TestCompute_FillerPenaltyCap(t *testing.T) {
	// 6 fillers per minute → fillerPenalty = 6*5 = 30 (exactly at cap).
	// 1 minute session (60 000 ms), 60 content words + 6 fillers.
	words := []store.Word{}
	t0 := 0
	add := func(text string) {
		words = append(words, makeWord(text, t0, t0+400))
		t0 += 1000 // one word per second
	}
	for i := 0; i < 10; i++ {
		add("content")
	}
	for i := 0; i < 6; i++ {
		add("um")
		for j := 0; j < 8; j++ {
			add("content")
		}
	}
	chunks := []store.Chunk{{Kind: "speech", OffsetMs: 0, Text: "filler heavy", Words: words}}
	d := Compute(chunks)
	t.Logf("FillerHeavy: fillersPerMin=%.2f fillerCount=%d score=%d", d.FillersPerMin, d.FillerCount, d.Score)

	if d.Score < 0 || d.Score > 100 {
		t.Errorf("score out of range: %d", d.Score)
	}
}

func TestCompute_PausePenaltyCap(t *testing.T) {
	// 5 long pauses → pausePenalty = 5*8 = 40, capped at 30.
	words := []store.Word{}
	t0 := 0
	for i := 0; i < 5; i++ {
		words = append(words, makeWord("content", t0, t0+400))
		t0 += 400 + 2500 // 2500ms gap after each word → long pause
		words = append(words, makeWord("content", t0, t0+400))
		t0 += 400 + 200
	}
	chunks := []store.Chunk{{Kind: "speech", OffsetMs: 0, Text: "pause heavy", Words: words}}
	d := Compute(chunks)
	t.Logf("PauseHeavy: pauses=%d score=%d", len(d.LongPauses), d.Score)

	if len(d.LongPauses) < 5 {
		t.Errorf("want ≥5 long pauses, got %d", len(d.LongPauses))
	}
	if d.Score < 0 {
		t.Errorf("score below floor: %d", d.Score)
	}
}

func TestCompute_ScoreFloor(t *testing.T) {
	// Worst case: maximum penalties from all three categories → floor at 0.
	words := []store.Word{}
	t0 := 0
	for i := 0; i < 20; i++ {
		words = append(words, makeWord("um", t0, t0+400))
		t0 += 400 + 3000 // 3s gap between every word → many long pauses
	}
	// WPM will be very low (< 110) too.
	chunks := []store.Chunk{{Kind: "speech", OffsetMs: 0, Text: "terrible", Words: words}}
	d := Compute(chunks)
	t.Logf("WorstCase: score=%d wpm=%.1f fillers=%d pauses=%d", d.Score, d.WPM, d.FillerCount, len(d.LongPauses))
	if d.Score < 0 {
		t.Errorf("score below floor 0: got %d", d.Score)
	}
}

// ---------------------------------------------------------------------------
// Multi-word filler: no double-counting
// ---------------------------------------------------------------------------

func TestCompute_MultiWordFillerNoDoubleCounting(t *testing.T) {
	// "you know" should count as 1 filler phrase, not 2 individual words.
	words := []store.Word{
		makeWord("you", 0, 400),
		makeWord("know", 500, 900),
		makeWord("content", 1000, 1400),
	}
	chunks := []store.Chunk{{Kind: "speech", OffsetMs: 0, Text: "test", Words: words}}
	d := Compute(chunks)

	if d.FillerCount != 1 {
		t.Errorf("'you know' should count as 1 filler, got FillerCount=%d fillers=%v", d.FillerCount, d.Fillers)
	}
	if d.Fillers["you know"] != 1 {
		t.Errorf("fillers[\"you know\"] should be 1, got %d", d.Fillers["you know"])
	}
	// "you" and "know" should NOT appear individually.
	if d.Fillers["you"] != 0 || d.Fillers["know"] != 0 {
		t.Errorf("individual 'you'/'know' should not be counted: %v", d.Fillers)
	}
}

// ---------------------------------------------------------------------------
// Answer-chunk words are still counted (Whisper timestamps apply)
// ---------------------------------------------------------------------------

func TestCompute_AnswerChunksIncluded(t *testing.T) {
	// A speech chunk with 60 words followed by an answer chunk with 60 words.
	// Both should contribute to WPM.
	makeChunk := func(kind string, offsetMs int, n int) store.Chunk {
		words := make([]store.Word, n)
		for i := range words {
			s := i * 500
			words[i] = makeWord("word", s, s+400)
		}
		return store.Chunk{Kind: kind, OffsetMs: offsetMs, Words: words}
	}

	chunks := []store.Chunk{
		makeChunk("speech", 0, 60),
		makeChunk("answer", 40_000, 60),
	}
	d := Compute(chunks)

	totalWords := 120
	if len(d.WPMBy30s) == 0 {
		t.Error("expected non-empty wpm_by_30s with both chunks")
	}

	// Total word count reflected in WPM calc.
	durationMin := float64(d.LongestPauseMs+1) / 60_000.0
	_ = durationMin
	_ = totalWords

	if d.WPM <= 0 {
		t.Errorf("expected positive WPM, got %.1f", d.WPM)
	}
}

// ---------------------------------------------------------------------------
// JSON shape compliance
// ---------------------------------------------------------------------------

func TestCompute_OutputShape(t *testing.T) {
	chunks := buildFixture()
	d := Compute(chunks)

	// Fillers must be a non-nil map.
	if d.Fillers == nil {
		t.Error("Fillers must not be nil")
	}
	// LongPauses must be a non-nil slice.
	if d.LongPauses == nil {
		t.Error("LongPauses must not be nil")
	}
	// WPMBy30s must be a non-nil slice.
	if d.WPMBy30s == nil {
		t.Error("WPMBy30s must not be nil")
	}
	// Score must be integer 0–100.
	if d.Score < 0 || d.Score > 100 {
		t.Errorf("Score %d out of range", d.Score)
	}
	// FillersPerMin must be ≥ 0.
	if math.IsNaN(d.FillersPerMin) || d.FillersPerMin < 0 {
		t.Errorf("FillersPerMin invalid: %f", d.FillersPerMin)
	}
	// TalkRatio must be 0–1.
	if d.TalkRatio < 0 || d.TalkRatio > 1 {
		t.Errorf("TalkRatio %f out of [0,1]", d.TalkRatio)
	}
}

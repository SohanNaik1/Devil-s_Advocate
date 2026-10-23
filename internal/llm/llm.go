// Package llm provides the LLM provider abstraction and interruption logic.
//
// Select a provider via the MODEL_PROVIDER environment variable:
//
//	groq        – Groq API (OpenAI-compatible), uses GROQ_API_KEY + GROQ_MODEL_ID
//	aistudio    – Google AI Studio (Gemini), uses GOOGLE_AI_STUDIO_API_KEY + GEMMA_MODEL_ID
//	fake        – always returns an interrupt with a wrong quote (tests quote verification)
//	fake_valid  – always returns an interrupt whose quote IS in the transcript (tests timing rule)
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// System prompt
// ---------------------------------------------------------------------------

const systemPrompt = `You are a critical AI panelist listening to a live presentation.
Analyze the transcript chunk below and decide whether to interrupt the speaker.

INTERRUPT ONLY for one of these trigger types:
  unsupported_claim  – a bold quantitative claim stated without any evidence
  contradiction      – directly contradicts something stated in the transcript
  jargon             – an unexplained technical term the audience likely does not know
  vague              – a statement so vague it conveys nothing meaningful

OUTPUT RULES (strictly enforced):
• Respond with ONLY a single JSON object. No markdown. No prose. No code fences.
• If you should NOT interrupt:
    {"interrupt": false}
• If you SHOULD interrupt:
    {"interrupt": true, "trigger": "<type>", "quote": "<verbatim excerpt>", "question": "<challenge under 25 words>"}

CRITICAL CONSTRAINTS:
1. "quote" must be copied VERBATIM, character-for-character, from the transcript.
2. "question" must be a direct, pointed challenge under 25 words.
3. Only interrupt if you are confident. Prefer {"interrupt": false} when in doubt.`

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// Decision is the parsed result of an LLM interrupt check.
type Decision struct {
	Interrupt bool   // true → the LLM wants to interrupt
	Trigger   string // "unsupported_claim" | "contradiction" | "jargon" | "vague"
	Quote     string // verbatim excerpt from the transcript
	Question  string // the panelist's challenge (< 25 words)
}

// Provider checks a transcript chunk and returns an interruption decision,
// or performs presentation evaluation.
type Provider interface {
	Decide(transcript string) (*Decision, error)
	Evaluate(prompt string) (string, error)
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

// New returns the Provider configured by MODEL_PROVIDER env var.
func New() (Provider, error) {
	p := os.Getenv("MODEL_PROVIDER")
	switch p {
	case "groq":
		return newGroq(), nil
	case "aistudio":
		return newAIStudio(), nil
	case "fake":
		return &fakeProvider{}, nil
	case "fake_valid":
		return &fakeValidProvider{}, nil
	default:
		return nil, fmt.Errorf("llm: unknown MODEL_PROVIDER %q (supported: groq, aistudio, fake, fake_valid)", p)
	}
}

// ---------------------------------------------------------------------------
// Shared: JSON parsing
// ---------------------------------------------------------------------------

// rawDecision is the wire format from the LLM.
type rawDecision struct {
	Interrupt bool   `json:"interrupt"`
	Trigger   string `json:"trigger,omitempty"`
	Quote     string `json:"quote,omitempty"`
	Question  string `json:"question,omitempty"`
}

// parseDecision parses LLM output into a Decision.
// It strips markdown code fences before parsing.
func parseDecision(raw string) (*Decision, error) {
	raw = strings.TrimSpace(raw)

	// Strip ``` fences: ```json\n...\n``` or ```\n...\n```
	if strings.HasPrefix(raw, "```") {
		// Drop the first line (the fence line) and the trailing ```
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
		raw = strings.TrimSpace(raw)
	}

	var rd rawDecision
	if err := json.Unmarshal([]byte(raw), &rd); err != nil {
		return nil, fmt.Errorf("parse JSON: %w (raw: %.200s)", err, raw)
	}
	if !rd.Interrupt {
		return &Decision{Interrupt: false}, nil
	}
	// Validate all required interrupt fields are present.
	if rd.Trigger == "" || rd.Quote == "" || rd.Question == "" {
		return nil, fmt.Errorf("incomplete interrupt JSON: missing trigger/quote/question (raw: %.200s)", raw)
	}
	return &Decision{
		Interrupt: true,
		Trigger:   rd.Trigger,
		Quote:     rd.Quote,
		Question:  rd.Question,
	}, nil
}

// ---------------------------------------------------------------------------
// Shared: retry wrapper
// ---------------------------------------------------------------------------

// decideWithRetry calls callFn, attempts to parse the result, and retries
// exactly once on invalid JSON. If both attempts fail, returns {Interrupt:false}.
func decideWithRetry(callFn func() (string, error)) (*Decision, error) {
	for attempt := 1; attempt <= 2; attempt++ {
		raw, err := callFn()
		if err != nil {
			if attempt == 2 {
				log.Printf("llm: both HTTP attempts failed: %v — returning no-interrupt", err)
				return &Decision{Interrupt: false}, nil
			}
			log.Printf("llm: attempt %d HTTP error (%v), retrying…", attempt, err)
			continue
		}

		d, err := parseDecision(raw)
		if err != nil {
			if attempt == 2 {
				log.Printf("llm: both attempts produced invalid JSON (%v) — returning no-interrupt", err)
				return &Decision{Interrupt: false}, nil
			}
			log.Printf("llm: attempt %d invalid JSON (%v), retrying…", attempt, err)
			continue
		}

		if attempt == 2 {
			log.Printf("llm: retry succeeded")
		}
		return d, nil
	}
	return &Decision{Interrupt: false}, nil
}

// ---------------------------------------------------------------------------
// Groq provider (OpenAI-compatible endpoint)
// ---------------------------------------------------------------------------

type groqProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func newGroq() *groqProvider {
	model := os.Getenv("GROQ_MODEL_ID")
	if model == "" {
		model = "qwen/qwen3.8-27b"
	}
	return &groqProvider{
		apiKey: os.Getenv("GROQ_API_KEY"),
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *groqProvider) Decide(transcript string) (*Decision, error) {
	return decideWithRetry(func() (string, error) { return g.callAPI(transcript) })
}

func (g *groqProvider) callAPI(transcript string) (string, error) {
	reqBody := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Transcript:\n" + transcript},
		},
		"temperature":     0.1,
		"max_tokens":      512,
		"response_format": map[string]string{"type": "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("groq: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.groq.com/openai/v1/chat/completions",
		bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("groq: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("User-Agent", "DevilsAdvocate/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq: http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq: status %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("groq: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("groq: empty choices in response")
	}
	return out.Choices[0].Message.Content, nil
}

func (g *groqProvider) Evaluate(prompt string) (string, error) {
	reqBody := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are an expert presentation evaluator. Return ONLY valid JSON."},
			{"role": "user", "content": prompt},
		},
		"temperature":     0.2,
		"max_tokens":      1024,
		"response_format": map[string]string{"type": "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("groq: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.groq.com/openai/v1/chat/completions",
		bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("groq: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("User-Agent", "DevilsAdvocate/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq: http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq: status %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("groq: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("groq: empty choices in response")
	}
	return out.Choices[0].Message.Content, nil
}

// ---------------------------------------------------------------------------
// AI Studio provider (Google Gemini API)
// ---------------------------------------------------------------------------

type aiStudioProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func newAIStudio() *aiStudioProvider {
	model := os.Getenv("GEMMA_MODEL_ID")
	if model == "" {
		model = "gemini-1.5-flash"
	}
	return &aiStudioProvider{
		apiKey: os.Getenv("GOOGLE_AI_STUDIO_API_KEY"),
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *aiStudioProvider) Decide(transcript string) (*Decision, error) {
	return decideWithRetry(func() (string, error) { return a.callAPI(transcript) })
}

func (a *aiStudioProvider) callAPI(transcript string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		a.model, a.apiKey,
	)

	reqBody := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": "Transcript:\n" + transcript}},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.1,
			"maxOutputTokens":  256,
			"responseMimeType": "application/json",
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("aistudio: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("aistudio: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("aistudio: http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("aistudio: status %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("aistudio: decode: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("aistudio: empty response")
	}
	return out.Candidates[0].Content.Parts[0].Text, nil
}

func (a *aiStudioProvider) Evaluate(prompt string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		a.model, a.apiKey,
	)

	reqBody := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": "You are an expert presentation evaluator. Return ONLY valid JSON."}},
		},
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": prompt}},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.2,
			"maxOutputTokens":  1024,
			"responseMimeType": "application/json",
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("aistudio: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("aistudio: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("aistudio: http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("aistudio: status %d: %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("aistudio: decode: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("aistudio: empty response")
	}
	return out.Candidates[0].Content.Parts[0].Text, nil
}

// ---------------------------------------------------------------------------
// fake provider — quote NEVER in transcript (tests quote verification failure)
// ---------------------------------------------------------------------------

type fakeProvider struct{}

func (f *fakeProvider) Decide(_ string) (*Decision, error) {
	return &Decision{
		Interrupt: true,
		Trigger:   "unsupported_claim",
		Quote:     "FAKE_QUOTE_THAT_DOES_NOT_EXIST_IN_ANY_TRANSCRIPT",
		Question:  "Can you back that fabricated claim with data?",
	}, nil
}

func (f *fakeProvider) Evaluate(_ string) (string, error) {
	return `{"score": 85, "structure": 88, "clarity": 84, "evidence": 80, "answers": 86, "went_well": [{"point": "Clear presentation framework", "quote": "our approach is ten times faster"}], "to_fix": [{"point": "Quantify speed benchmarks", "quote": "ten times faster", "suggestion": "Cite actual latency or throughput numbers"}]}`, nil
}

// ---------------------------------------------------------------------------
// fake_valid provider — quote IS the transcript (tests 60-second timing rule)
// ---------------------------------------------------------------------------

type fakeValidProvider struct{}

func (f *fakeValidProvider) Decide(transcript string) (*Decision, error) {
	return &Decision{
		Interrupt: true,
		Trigger:   "unsupported_claim",
		Quote:     transcript,
		Question:  "Can you back this claim with concrete evidence?",
	}, nil
}

func (f *fakeValidProvider) Evaluate(_ string) (string, error) {
	return `{"score": 85, "structure": 88, "clarity": 84, "evidence": 80, "answers": 86, "went_well": [{"point": "Clear presentation framework", "quote": "our approach is ten times faster"}], "to_fix": [{"point": "Quantify speed benchmarks", "quote": "ten times faster", "suggestion": "Cite actual latency or throughput numbers"}]}`, nil
}

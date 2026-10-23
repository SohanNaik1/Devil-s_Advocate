package llm

import (
	"errors"
	"os"
	"testing"

	"github.com/joho/godotenv"
)

func TestParseDecision(t *testing.T) {
	t.Run("valid no interrupt", func(t *testing.T) {
		raw := `{"interrupt": false}`
		d, err := parseDecision(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if d.Interrupt {
			t.Fatalf("expected Interrupt=false, got true")
		}
	})

	t.Run("valid interrupt", func(t *testing.T) {
		raw := `{"interrupt": true, "trigger": "unsupported_claim", "quote": "ten times faster", "question": "Where is the data?"}`
		d, err := parseDecision(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !d.Interrupt {
			t.Fatalf("expected Interrupt=true, got false")
		}
		if d.Trigger != "unsupported_claim" || d.Quote != "ten times faster" || d.Question != "Where is the data?" {
			t.Fatalf("unexpected fields: %+v", d)
		}
	})

	t.Run("valid interrupt with markdown code fences", func(t *testing.T) {
		raw := "```json\n{\"interrupt\": true, \"trigger\": \"jargon\", \"quote\": \"quantum flux\", \"question\": \"What is quantum flux?\"}\n```"
		d, err := parseDecision(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !d.Interrupt || d.Trigger != "jargon" {
			t.Fatalf("unexpected fields: %+v", d)
		}
	})

	t.Run("incomplete interrupt missing quote", func(t *testing.T) {
		raw := `{"interrupt": true, "trigger": "vague", "question": "Can you be specific?"}`
		_, err := parseDecision(raw)
		if err == nil {
			t.Fatalf("expected error for missing quote, got nil")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		raw := `this is not json at all`
		_, err := parseDecision(raw)
		if err == nil {
			t.Fatalf("expected error for malformed json, got nil")
		}
	})
}

func TestDecideWithRetry(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		attempts := 0
		d, err := decideWithRetry(func() (string, error) {
			attempts++
			return `{"interrupt": false}`, nil
		})
		if err != nil || d.Interrupt || attempts != 1 {
			t.Fatalf("expected 1 attempt and no interrupt, got attempts=%d, d=%+v, err=%v", attempts, d, err)
		}
	})

	t.Run("retries on malformed JSON and succeeds on second attempt", func(t *testing.T) {
		attempts := 0
		d, err := decideWithRetry(func() (string, error) {
			attempts++
			if attempts == 1 {
				return "bad json", nil
			}
			return `{"interrupt": true, "trigger": "vague", "quote": "maybe", "question": "Are you sure?"}`, nil
		})
		if err != nil || !d.Interrupt || attempts != 2 {
			t.Fatalf("expected 2 attempts and interrupt=true, got attempts=%d, d=%+v, err=%v", attempts, d, err)
		}
	})

	t.Run("falls back to no-interrupt after two failed attempts", func(t *testing.T) {
		attempts := 0
		d, err := decideWithRetry(func() (string, error) {
			attempts++
			return "still bad json", nil
		})
		if err != nil {
			t.Fatalf("expected nil error on fallback, got %v", err)
		}
		if d.Interrupt || attempts != 2 {
			t.Fatalf("expected 2 attempts and interrupt=false fallback, got attempts=%d, d=%+v", attempts, d)
		}
	})

	t.Run("falls back to no-interrupt on network errors", func(t *testing.T) {
		attempts := 0
		d, err := decideWithRetry(func() (string, error) {
			attempts++
			return "", errors.New("network timeout")
		})
		if err != nil {
			t.Fatalf("expected nil error on fallback, got %v", err)
		}
		if d.Interrupt || attempts != 2 {
			t.Fatalf("expected 2 attempts and fallback, got attempts=%d, d=%+v", attempts, d)
		}
	})
}

func TestFakeProviders(t *testing.T) {
	fake := &fakeProvider{}
	d, err := fake.Decide("hello world")
	if err != nil || !d.Interrupt {
		t.Fatalf("fake provider failed: %+v, %v", d, err)
	}
	if d.Quote == "hello world" {
		t.Fatalf("fake provider quote should not match input")
	}

	fakeValid := &fakeValidProvider{}
	d2, err2 := fakeValid.Decide("exact transcript text")
	if err2 != nil || !d2.Interrupt {
		t.Fatalf("fakeValid provider failed: %+v, %v", d2, err2)
	}
	if d2.Quote != "exact transcript text" {
		t.Fatalf("fakeValid provider quote should exactly match input")
	}
}

func TestGroqProviderLive(t *testing.T) {
	// Try loading .env from project root
	_ = os.Chdir("../..")
	_ = godotenv.Load()
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		t.Skip("skipping live Groq test: GROQ_API_KEY not set in env")
	}

	os.Setenv("MODEL_PROVIDER", "groq")
	p, err := New()
	if err != nil {
		t.Fatalf("failed to create Groq provider: %v", err)
	}

	claim := "our approach is 10 times faster than anything else on the market."
	decision, err := p.Decide(claim)
	if err != nil {
		t.Fatalf("Groq decide failed: %v", err)
	}
	if !decision.Interrupt {
		t.Fatalf("expected interrupt=true on bold claim, got false")
	}
	if decision.Trigger == "" || decision.Quote == "" || decision.Question == "" {
		t.Fatalf("incomplete decision fields: %+v", decision)
	}
	t.Logf("Groq decision: trigger=%s quote=%q question=%q", decision.Trigger, decision.Quote, decision.Question)
}

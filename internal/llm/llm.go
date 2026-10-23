// Package llm provides the LLM provider abstraction.
// Switch between providers via the MODEL_PROVIDER environment variable.
package llm

import (
	"fmt"
	"os"
)

// Provider is the interface every LLM backend must implement.
type Provider interface {
	// Complete sends a prompt and returns the model's response text.
	Complete(prompt string) (string, error)
}

// New returns a Provider based on the MODEL_PROVIDER env var.
//
//	"aistudio"    → Google AI Studio (Gemini / Gemma via API)
//	"gemma-local" → locally hosted Gemma (future)
func New() (Provider, error) {
	switch p := os.Getenv("MODEL_PROVIDER"); p {
	case "aistudio":
		return &aiStudioProvider{}, nil
	case "gemma-local":
		return &gemmaLocalProvider{}, nil
	default:
		return nil, fmt.Errorf("llm: unknown MODEL_PROVIDER %q", p)
	}
}

// ---------------------------------------------------------------------------
// Stubs — real implementations come in later phases.
// ---------------------------------------------------------------------------

type aiStudioProvider struct{}

func (a *aiStudioProvider) Complete(prompt string) (string, error) {
	return "", fmt.Errorf("aiStudioProvider: not yet implemented")
}

type gemmaLocalProvider struct{}

func (g *gemmaLocalProvider) Complete(prompt string) (string, error) {
	return "", fmt.Errorf("gemmaLocalProvider: not yet implemented")
}

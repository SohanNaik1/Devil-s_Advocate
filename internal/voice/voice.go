// Package voice wraps the ElevenLabs Text-to-Speech API.
//
// Usage:
//
//	client := voice.New()
//	audioBytes, err := client.Speak("What evidence supports that claim?")
//
// Environment variables required:
//
//	ELEVENLABS_API_KEY         – your ElevenLabs API key
//	ELEVENLABS_VOICE_ID_PANELIST – voice ID for the AI panelist (default: pNInz6obpgDQGcFmaJgB "Adam")
package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client calls ElevenLabs TTS and caches the resulting audio bytes in memory.
type Client struct {
	apiKey  string
	voiceID string
	httpCl  *http.Client

	// audioCache stores id → mp3 bytes so GET /api/audio/{id}.mp3 can serve them.
	mu         sync.RWMutex
	audioCache map[string][]byte
}

// New returns a Client configured from environment variables.
// Falls back to sensible defaults when variables are missing.
func New() *Client {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	voiceID := os.Getenv("ELEVENLABS_VOICE_ID_PANELIST")
	if voiceID == "" {
		voiceID = "pNInz6obpgDQGcFmaJgB" // Adam – lowest latency free voice
	}
	if apiKey == "" {
		log.Println("voice: WARNING — ELEVENLABS_API_KEY not set; TTS will be unavailable")
	}
	return &Client{
		apiKey:     apiKey,
		voiceID:    voiceID,
		httpCl:     &http.Client{Timeout: 20 * time.Second},
		audioCache: make(map[string][]byte),
	}
}

// Speak synthesises text via ElevenLabs, caches the MP3 under the given id,
// and returns the raw audio bytes.
func (c *Client) Speak(id, text string) ([]byte, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("voice: ELEVENLABS_API_KEY not configured")
	}

	body, err := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_turbo_v2_5", // lowest-latency streaming model
		"voice_settings": map[string]any{
			"stability":        0.45,
			"similarity_boost": 0.75,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("voice: marshal request: %w", err)
	}

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", c.voiceID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("voice: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := c.httpCl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voice: http request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voice: ElevenLabs returned %d: %s", resp.StatusCode, string(raw))
	}

	// Cache and return.
	c.mu.Lock()
	c.audioCache[id] = raw
	c.mu.Unlock()

	log.Printf("voice: synthesised %d bytes for interrupt %s", len(raw), id)
	return raw, nil
}

// GetAudio returns cached MP3 bytes for id.  Returns nil, false if not found.
func (c *Client) GetAudio(id string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, ok := c.audioCache[id]
	return data, ok
}

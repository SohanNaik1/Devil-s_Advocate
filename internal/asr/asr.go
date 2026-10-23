// Package asr provides an HTTP client for the Whisper transcription service.
package asr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// Word mirrors the word-level timestamp returned by the Whisper service.
type Word struct {
	Word    string `json:"word"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

// Response is the JSON shape returned by POST /transcribe.
type Response struct {
	Text       string `json:"text"`
	DurationMs int    `json:"duration_ms"`
	Words      []Word `json:"words"`
}

// Client calls the Whisper microservice.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// New returns a Client pointed at the Whisper service.
// endpoint defaults to http://localhost:9000 when empty.
func New(endpoint string) *Client {
	if endpoint == "" {
		endpoint = whisperEndpoint()
	}
	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // generous timeout for CPU inference
		},
	}
}

// whisperEndpoint reads WHISPER_URL from the environment, falling back to localhost.
func whisperEndpoint() string {
	if u := os.Getenv("WHISPER_URL"); u != "" {
		return u
	}
	return "http://localhost:9000"
}

// Transcribe sends audio bytes to the Whisper service and returns the transcript.
// filename is used as the multipart field filename (e.g. "chunk.wav").
func (c *Client) Transcribe(audio []byte, filename string) (*Response, error) {
	if filename == "" {
		filename = "audio.wav"
	}

	// Build multipart body with field name "audio" as the server expects.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("audio", filename)
	if err != nil {
		return nil, fmt.Errorf("asr: create form file: %w", err)
	}
	if _, err := io.Copy(fw, bytes.NewReader(audio)); err != nil {
		return nil, fmt.Errorf("asr: write audio bytes: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("asr: close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.endpoint+"/transcribe", &body)
	if err != nil {
		return nil, fmt.Errorf("asr: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asr: call whisper: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("asr: whisper returned %d: %s", resp.StatusCode, raw)
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("asr: decode response: %w", err)
	}
	return &result, nil
}

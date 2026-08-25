package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiProvider_BuildRequestBody(t *testing.T) {
	p := NewGeminiProvider("dummy-key", "gemini-3.6-flash")

	history := []Message{
		{Role: "user", Content: "こんにちは"},
		{Role: "assistant", Content: "こんにちは！何かお手伝いできますか？"},
	}
	systemPrompt := "あなたは公式アシスタントです。"
	prompt := "歴史を教えて"

	bodyBytes, err := p.buildRequestBody(systemPrompt, history, prompt)
	if err != nil {
		t.Fatalf("buildRequestBody failed: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "あなたは公式アシスタントです。") {
		t.Errorf("Expected systemPrompt in body, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "歴史を教えて") {
		t.Errorf("Expected user prompt in body, got: %s", bodyStr)
	}
	// "assistant" role should be converted to "model" for Gemini
	if !strings.Contains(bodyStr, `"role":"model"`) {
		t.Errorf("Expected role conversion to model, got: %s", bodyStr)
	}
}

func TestGeminiProvider_GenerateReply_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"candidates": [{
				"content": {
					"parts": [{"text": "モックGeminiからの回答です。"}]
				}
			}],
			"usageMetadata": {
				"promptTokenCount": 10,
				"candidatesTokenCount": 20,
				"totalTokenCount": 30
			}
		}`)
	}))
	defer server.Close()

	p := &GeminiProvider{
		APIKey:  "test-api-key",
		Model:   "gemini-test",
		BaseURL: server.URL,
		Client:  server.Client(),
	}

	reply, err := p.GenerateReply(context.Background(), "sys", nil, "hello")
	if err != nil {
		t.Fatalf("GenerateReply failed: %v", err)
	}
	if reply != "モックGeminiからの回答です。" {
		t.Errorf("Unexpected reply: %s", reply)
	}
}

func TestGeminiProvider_GenerateReply_Errors(t *testing.T) {
	// 1. Missing Key
	pNoKey := &GeminiProvider{APIKey: ""}
	_, err := pNoKey.GenerateReply(context.Background(), "", nil, "test")
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY is not configured") {
		t.Errorf("Expected GEMINI_API_KEY error, got: %v", err)
	}

	// 2. API Error JSON returned
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"Invalid argument","code":400}}`)
	}))
	defer errServer.Close()

	pErr := &GeminiProvider{
		APIKey:  "test-key",
		BaseURL: errServer.URL,
		Client:  errServer.Client(),
	}
	_, err = pErr.GenerateReply(context.Background(), "", nil, "test")
	if err == nil || !strings.Contains(err.Error(), "Invalid argument") {
		t.Errorf("Expected API error, got: %v", err)
	}

	// 3. Empty Candidates
	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"candidates":[]}`)
	}))
	defer emptyServer.Close()

	pEmpty := &GeminiProvider{
		APIKey:  "test-key",
		BaseURL: emptyServer.URL,
		Client:  emptyServer.Client(),
	}
	_, err = pEmpty.GenerateReply(context.Background(), "", nil, "test")
	if err == nil || !strings.Contains(err.Error(), "no reply generated") {
		t.Errorf("Expected no reply error, got: %v", err)
	}
}

func TestGeminiProvider_GenerateReplyStream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)

		fmt.Fprintf(w, "data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"チャンク1\"}]}}], \"usageMetadata\": {\"totalTokenCount\": 10}}\n\n")
		flusher.Flush()
		time.Sleep(5 * time.Millisecond)

		fmt.Fprintf(w, "data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"チャンク2\"}]}}]}\n\n")
		flusher.Flush()
		time.Sleep(5 * time.Millisecond)

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := &GeminiProvider{
		APIKey:  "test-key",
		Model:   "gemini-test",
		BaseURL: server.URL,
		Client:  server.Client(),
	}

	var chunks []string
	err := p.GenerateReplyStream(context.Background(), "sys", nil, "test", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("GenerateReplyStream failed: %v", err)
	}

	if len(chunks) != 2 || chunks[0] != "チャンク1" || chunks[1] != "チャンク2" {
		t.Errorf("Unexpected chunks: %v", chunks)
	}
}

func TestGeminiProvider_GenerateReplyStream_Errors(t *testing.T) {
	// 1. Missing Key
	pNoKey := &GeminiProvider{APIKey: ""}
	err := pNoKey.GenerateReplyStream(context.Background(), "", nil, "test", func(chunk string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY is not configured") {
		t.Errorf("Expected missing key error, got: %v", err)
	}

	// 2. HTTP Error Status with JSON Error
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"Bad stream request","code":400}}`)
	}))
	defer errServer.Close()

	pErr := &GeminiProvider{
		APIKey:  "test-key",
		BaseURL: errServer.URL,
		Client:  errServer.Client(),
	}
	err = pErr.GenerateReplyStream(context.Background(), "", nil, "test", func(chunk string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "Bad stream request") {
		t.Errorf("Expected stream API error, got: %v", err)
	}

	// 3. Streaming chunk contains error
	chunkErrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"error\": {\"message\": \"Quota exceeded\", \"code\": 429}}\n\n")
		flusher.Flush()
	}))
	defer chunkErrServer.Close()

	pChunkErr := &GeminiProvider{
		APIKey:  "test-key",
		BaseURL: chunkErrServer.URL,
		Client:  chunkErrServer.Client(),
	}
	err = pChunkErr.GenerateReplyStream(context.Background(), "", nil, "test", func(chunk string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "Quota exceeded") {
		t.Errorf("Expected Quota exceeded error, got: %v", err)
	}
}

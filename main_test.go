package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hatrot/trot-web/ai"
)

func TestChatHandler(t *testing.T) {
	// 1. Invalid method (GET)
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	w := httptest.NewRecorder()
	chatHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed, got %d", w.Code)
	}

	// 2. Options (CORS preflight)
	req = httptest.NewRequest(http.MethodOptions, "/api/chat", nil)
	w = httptest.NewRecorder()
	chatHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected CORS origin header")
	}

	// 3. Valid POST Question (Fallback to Mock if GEMINI_API_KEY is not set)
	body, _ := json.Marshal(chatRequest{Question: "トロットの歴史について"})
	req = httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	chatHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp chatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(resp.Reply) == 0 {
		t.Errorf("Expected non-empty reply")
	}
	if !strings.Contains(resp.Reply, "1984") && !strings.Contains(resp.Reply, "1993") && !strings.Contains(resp.Reply, "トロット") {
		t.Errorf("Expected relevant history reply, got: %s", resp.Reply)
	}
}

func TestChatStreamHandler(t *testing.T) {
	// 1. Invalid method (GET)
	req := httptest.NewRequest(http.MethodGet, "/api/chat-stream", nil)
	w := httptest.NewRecorder()
	chatStreamHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed, got %d", w.Code)
	}

	// 2. Options (CORS preflight)
	req = httptest.NewRequest(http.MethodOptions, "/api/chat-stream", nil)
	w = httptest.NewRecorder()
	chatStreamHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected CORS origin header")
	}

	// 3. Valid POST Stream Request (SSE)
	body, _ := json.Marshal(chatRequest{Question: "トロットの歴史について"})
	req = httptest.NewRequest(http.MethodPost, "/api/chat-stream", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	chatStreamHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", w.Header().Get("Content-Type"))
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "data:") {
		t.Errorf("Expected SSE data lines, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Errorf("Expected stream end with [DONE], got: %s", bodyStr)
	}
}

func TestContactHandler(t *testing.T) {
	// 1. Invalid method (GET)
	req := httptest.NewRequest(http.MethodGet, "/api/contact", nil)
	w := httptest.NewRecorder()
	contactHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed, got %d", w.Code)
	}

	// 2. Empty Body
	req = httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader("{}"))
	w = httptest.NewRecorder()
	contactHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for empty body, got %d", w.Code)
	}

	// 3. Valid POST
	body, _ := json.Marshal(contactRequest{
		Name:    "テストユーザー",
		Email:   "test@example.com",
		Content: "クラウド移行のご相談",
		Source:  "Unit Test",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/contact", bytes.NewReader(body))
	w = httptest.NewRecorder()
	contactHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp contactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("Expected status ok, got %s", resp.Status)
	}
}

func TestHealthAndWarmupHandlers(t *testing.T) {
	// 1. Warmup
	req := httptest.NewRequest(http.MethodGet, "/_ah/warmup", nil)
	w := httptest.NewRecorder()
	warmupHandler(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "warmup done" {
		t.Errorf("Expected 200 'warmup done' for warmupHandler, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Health check (Plain text)
	req = httptest.NewRequest(http.MethodGet, "/_ah/health", nil)
	w = httptest.NewRecorder()
	healthCheckHandler(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Errorf("Expected 200 ok for healthCheck, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Health check (JSON format)
	req = httptest.NewRequest(http.MethodGet, "/_ah/health?format=json", nil)
	w = httptest.NewRecorder()
	healthCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for healthCheck JSON, got %d", w.Code)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("Failed to parse healthCheck JSON: %v", err)
	}
	if data["status"] != "ok" || data["ready"] != true {
		t.Errorf("Expected ready true, got: %v", data)
	}
}

func TestChatHandler_ErrorCases(t *testing.T) {
	// 1. Invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader("{invalid-json"))
	w := httptest.NewRecorder()
	chatHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid JSON, got %d", w.Code)
	}

	// 2. Empty question
	req = httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"question": ""}`))
	w = httptest.NewRecorder()
	chatHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty question, got %d", w.Code)
	}
}

func TestChatStreamHandler_ErrorCases(t *testing.T) {
	// 1. Invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/chat-stream", strings.NewReader("{invalid-json"))
	w := httptest.NewRecorder()
	chatStreamHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid JSON, got %d", w.Code)
	}

	// 2. Empty question
	req = httptest.NewRequest(http.MethodPost, "/api/chat-stream", strings.NewReader(`{"question": "  "}`))
	w = httptest.NewRecorder()
	chatStreamHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for whitespace-only question, got %d", w.Code)
	}
}

func TestIndexHandler_LocalFiles(t *testing.T) {
	// 1. Root / -> serves www/index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	indexHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for root index, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "H.A.Trot") {
		t.Errorf("Expected index content containing 'H.A.Trot'")
	}

	// 2. JS file -> serves www/js/chat.js
	req = httptest.NewRequest(http.MethodGet, "/js/chat.js", nil)
	w = httptest.NewRecorder()
	indexHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for /js/chat.js, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "trot-chat") {
		t.Errorf("Expected chat.js content")
	}
}

func TestRedirectHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/redirect", nil)
	w := httptest.NewRecorder()
	redirectHandler(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("Expected 302 Found, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "youtube.com") {
		t.Errorf("Expected youtube location, got %s", w.Header().Get("Location"))
	}
}

func TestHealthCheckHandler_Unhealthy(t *testing.T) {
	// Temporarily unset provider to make service unready
	svc := ai.GetService()
	svc.SetProvider(nil)

	req := httptest.NewRequest(http.MethodGet, "/_ah/health", nil)
	w := httptest.NewRecorder()
	healthCheckHandler(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 Service Unavailable for unready service, got %d", w.Code)
	}

	// JSON Unhealthy
	req = httptest.NewRequest(http.MethodGet, "/_ah/health?format=json", nil)
	w = httptest.NewRecorder()
	healthCheckHandler(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 JSON for unready service, got %d", w.Code)
	}

	// Restore service
	svc.SetProvider(ai.NewMockProvider())
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

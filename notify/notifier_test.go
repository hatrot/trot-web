package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCompositeNotifier_LogFallback(t *testing.T) {
	// Clear env vars
	os.Unsetenv("SLACK_WEBHOOK_URL")
	os.Unsetenv("SENDGRID_API_KEY")

	notifier := NewNotifier()
	if notifier == nil {
		t.Fatalf("Expected non-nil notifier")
	}
	if notifier.Name() != "Log" {
		t.Errorf("Expected fallback to 'Log', got: %s", notifier.Name())
	}

	msg := ContactMessage{
		Name:    "テスト太郎",
		Email:   "taro@example.com",
		Content: "テストご相談内容",
		Source:  "Unit Test",
	}

	ctx := context.Background()
	if err := notifier.Send(ctx, msg); err != nil {
		t.Errorf("Expected successful send with fallback, got: %v", err)
	}
}

func TestSlackNotifier_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		client:     server.Client(),
	}

	if notifier.Name() != "Slack" {
		t.Errorf("Expected 'Slack', got: %s", notifier.Name())
	}

	msg := ContactMessage{
		Name:    "山田花子",
		Email:   "hanako@example.com",
		Content: "システムリプレイスのご相談",
		Source:  "AI Q&A",
	}

	err := notifier.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	text, ok := receivedBody["text"].(string)
	if !ok || !strings.Contains(text, "山田花子") || !strings.Contains(text, "hanako@example.com") {
		t.Errorf("Slack payload missing expected info: %v", receivedBody)
	}
}

func TestSlackNotifier_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal error")
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		client:     server.Client(),
	}

	err := notifier.Send(context.Background(), ContactMessage{Name: "テスト"})
	if err == nil || !strings.Contains(err.Error(), "slack webhook error: status 500") {
		t.Errorf("Expected 500 error, got: %v", err)
	}
}

func TestSendGridNotifier_Success(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	notifier := &SendGridNotifier{
		apiKey:    "SG.test_key",
		fromEmail: "from@example.com",
		toEmail:   "to@example.com",
		apiURL:    server.URL,
		client:    server.Client(),
	}

	if notifier.Name() != "SendGrid" {
		t.Errorf("Expected 'SendGrid', got: %s", notifier.Name())
	}

	err := notifier.Send(context.Background(), ContactMessage{
		Name:    "テスト",
		Email:   "test@example.com",
		Content: "テスト内容",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if authHeader != "Bearer SG.test_key" {
		t.Errorf("Expected Bearer SG.test_key, got: %s", authHeader)
	}
}

func TestSendGridNotifier_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"errors":[{"message":"Bad request"}]}`)
	}))
	defer server.Close()

	notifier := &SendGridNotifier{
		apiKey:    "SG.test_key",
		fromEmail: "from@example.com",
		toEmail:   "to@example.com",
		apiURL:    server.URL,
		client:    server.Client(),
	}

	err := notifier.Send(context.Background(), ContactMessage{Name: "テスト"})
	if err == nil || !strings.Contains(err.Error(), "sendgrid error: status 400") {
		t.Errorf("Expected 400 error, got: %v", err)
	}
}

func TestCompositeNotifier_Multiple(t *testing.T) {
	logN := NewLogNotifier()
	comp := &CompositeNotifier{
		notifiers: []Notifier{logN},
	}

	if comp.Name() != "Log" {
		t.Errorf("Expected 'Log', got: %s", comp.Name())
	}

	err := comp.Send(context.Background(), ContactMessage{
		Name:    "テスト",
		Email:   "test@example.com",
		Content: "内容",
	})
	if err != nil {
		t.Errorf("Expected success, got: %v", err)
	}
}

func TestNewNotifier_WithEnvVars(t *testing.T) {
	os.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/services/test/test/test")
	os.Setenv("SENDGRID_API_KEY", "SG.test")
	os.Setenv("CONTACT_NOTIFY_EMAIL", "notify@example.com")
	defer func() {
		os.Unsetenv("SLACK_WEBHOOK_URL")
		os.Unsetenv("SENDGRID_API_KEY")
		os.Unsetenv("CONTACT_NOTIFY_EMAIL")
	}()

	n := NewNotifier()
	if !strings.Contains(n.Name(), "Slack") || !strings.Contains(n.Name(), "SendGrid") {
		t.Errorf("Expected Slack+SendGrid in notifier name, got: %s", n.Name())
	}

	sn := NewSlackNotifier("https://example.com")
	if sn == nil || sn.Name() != "Slack" {
		t.Errorf("Expected non-nil SlackNotifier")
	}

	sgn := NewSendGridNotifier("key", "", "to@example.com")
	if sgn == nil || sgn.fromEmail != "kato@trot.co.jp" {
		t.Errorf("Expected default fromEmail, got: %s", sgn.fromEmail)
	}
}

package notify

import (
	"context"
	"testing"
)

func TestCompositeNotifier_LogFallback(t *testing.T) {
	notifier := NewNotifier()
	if notifier == nil {
		t.Fatalf("Expected non-nil notifier")
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

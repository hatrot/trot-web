package ai

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMockProvider_GenerateReply_Branches(t *testing.T) {
	m := NewMockProvider()

	if m.Name() != "Mock (Offline Demo)" {
		t.Errorf("Expected 'Mock (Offline Demo)', got: %s", m.Name())
	}

	ctx := context.Background()

	// 1. History
	res, err := m.GenerateReply(ctx, "", nil, "会社の沿革や歴史について")
	if err != nil || !strings.Contains(res, "1993") {
		t.Errorf("Expected history response, got: %s (err: %v)", res, err)
	}

	// 2. Tech
	res, err = m.GenerateReply(ctx, "", nil, "得意な開発言語やスタック")
	if err != nil || !strings.Contains(res, "Go言語") {
		t.Errorf("Expected tech response, got: %s (err: %v)", res, err)
	}

	// 3. Product
	res, err = m.GenerateReply(ctx, "", nil, "NACK5TOUCHなどのプロダクト")
	if err != nil || !strings.Contains(res, "NACK5TOUCH") {
		t.Errorf("Expected product response, got: %s (err: %v)", res, err)
	}

	// 4. Default / Unknown
	res, err = m.GenerateReply(ctx, "", nil, "今日の天気はどうですか？")
	if err != nil || !strings.Contains(res, "公式アーカイブにはまだ記載のない未知の領域") {
		t.Errorf("Expected default response, got: %s (err: %v)", res, err)
	}
}

func TestMockProvider_GenerateReplyStream(t *testing.T) {
	m := NewMockProvider()
	ctx := context.Background()

	var chunks []string
	err := m.GenerateReplyStream(ctx, "", nil, "歴史を教えて", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("GenerateReplyStream failed: %v", err)
	}

	if len(chunks) <= 1 {
		t.Errorf("Expected multiple chunks from mock stream, got %d", len(chunks))
	}

	combined := strings.Join(chunks, "")
	expected, _ := m.GenerateReply(ctx, "", nil, "歴史を教えて")
	if combined != expected {
		t.Errorf("Combined stream does not match GenerateReply. Expected:\n%s\nGot:\n%s", expected, combined)
	}
}

func TestMockProvider_GenerateReplyStream_ContextCancel(t *testing.T) {
	m := NewMockProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var count int
	_ = m.GenerateReplyStream(ctx, "", nil, "長い歴史を教えて", func(chunk string) error {
		count++
		return nil
	})

	// Stream should be interrupted by context timeout
	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got: %v", ctx.Err())
	}
}

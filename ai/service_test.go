package ai

import (
	"context"
	"strings"
	"testing"
)

func TestService_KnowledgeLoadingAndMockProvider(t *testing.T) {
	svc := NewService(NewMockProvider())
	err := svc.LoadKnowledge("../docs")
	if err != nil {
		t.Fatalf("Failed to load knowledge: %v", err)
	}

	if len(svc.knowledge) == 0 {
		t.Errorf("Expected knowledge to be loaded, got empty")
	}

	if !strings.Contains(svc.systemPrompt, "エイチ・エィ・トロット") {
		t.Errorf("System prompt does not contain expected company name")
	}

	ctx := context.Background()

	// Test History Question
	reply, provider, err := svc.Ask(ctx, nil, "トロットの歴史を教えて")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if !strings.Contains(reply, "1984年") || !strings.Contains(reply, "1993年") {
		t.Errorf("Expected history details in reply, got: %s", reply)
	}
	if provider != "Mock (Offline Demo)" {
		t.Errorf("Expected Mock provider name, got: %s", provider)
	}

	// Test Guardrail / Unknown Question
	reply, _, err = svc.Ask(ctx, nil, "社長の今日の晩御飯は何ですか？")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if !strings.Contains(reply, "公式アーカイブにはまだ記載のない未知の領域") {
		t.Errorf("Expected guardrail refusal for unknown question, got: %s", reply)
	}
}

func TestService_ProviderSwapping(t *testing.T) {
	svc := NewService(NewMockProvider())
	if svc.ProviderName() != "Mock (Offline Demo)" {
		t.Errorf("Expected Mock provider, got: %s", svc.ProviderName())
	}

	// Swap to Gemini provider
	gemini := NewGeminiProvider("test-key", "gemini-2.0-flash")
	svc.SetProvider(gemini)
	if svc.ProviderName() != "Gemini (gemini-2.0-flash)" {
		t.Errorf("Expected Gemini provider, got: %s", svc.ProviderName())
	}
}

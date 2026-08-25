package ai

import "context"

// Message represents a chat message in the conversation.
type Message struct {
	Role    string `json:"role"`    // "user" or "model" / "assistant"
	Content string `json:"content"` // message text
}

// Usage contains token consumption statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CandidatesTokens int `json:"candidates_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// StreamChunkHandler is called when a new text chunk is received from the LLM.
type StreamChunkHandler func(chunk string) error

// LLMProvider is the pluggable interface for LLM backends (Gemini, Claude, OpenAI, Mock, etc.).
type LLMProvider interface {
	// Name returns the provider's display or identifier name.
	Name() string
	// GenerateReply generates a reply using the given system prompt, conversation history, and current question.
	GenerateReply(ctx context.Context, systemPrompt string, history []Message, prompt string) (string, error)
	// GenerateReplyStream streams reply chunks using the given system prompt, conversation history, and current question.
	GenerateReplyStream(ctx context.Context, systemPrompt string, history []Message, prompt string, onChunk StreamChunkHandler) error
}

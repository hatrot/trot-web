package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// GeminiProvider implements LLMProvider using Google's Gemini REST API.
type GeminiProvider struct {
	APIKey string
	Model  string
	Client *http.Client
}

// NewGeminiProvider creates a new GeminiProvider.
// If apiKey is empty, it attempts to read from GEMINI_API_KEY environment variable.
func NewGeminiProvider(apiKey string, model string) *GeminiProvider {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = "gemini-3.6-flash"
		}
	}
	return &GeminiProvider{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiProvider) Name() string {
	return fmt.Sprintf("Gemini (%s)", g.Model)
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiReq struct {
	SystemInstruction *geminiContent  `json:"system_instruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  *geminiGenConf  `json:"generationConfig,omitempty"`
}

type geminiGenConf struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiResp struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (g *GeminiProvider) GenerateReply(ctx context.Context, systemPrompt string, history []Message, prompt string) (string, error) {
	if g.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not configured")
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.Model, g.APIKey)

	var contents []geminiContent
	for _, m := range history {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: prompt}},
	})

	bodyData := geminiReq{
		Contents: contents,
		GenerationConfig: &geminiGenConf{
			Temperature:     0.7,
			MaxOutputTokens: 800,
		},
	}
	if systemPrompt != "" {
		bodyData.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	payload, err := json.Marshal(bodyData)
	if err != nil {
		return "", fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response failed: %w", err)
	}

	var gResp geminiResp
	if err := json.Unmarshal(respBytes, &gResp); err != nil {
		return "", fmt.Errorf("unmarshal response failed: %w (status: %d)", err, resp.StatusCode)
	}

	if gResp.Error != nil {
		return "", fmt.Errorf("gemini api error: %s (code: %d)", gResp.Error.Message, gResp.Error.Code)
	}

	if gResp.UsageMetadata != nil {
		log.Printf("[Gemini Token Usage] Model: %s, Prompt: %d, Output: %d, Total: %d tokens",
			g.Model, gResp.UsageMetadata.PromptTokenCount, gResp.UsageMetadata.CandidatesTokenCount, gResp.UsageMetadata.TotalTokenCount)
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no reply generated from Gemini")
	}

	return gResp.Candidates[0].Content.Parts[0].Text, nil
}

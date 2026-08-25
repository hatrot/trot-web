package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// GeminiProvider implements LLMProvider using Google's Gemini REST API.
type GeminiProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
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
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://generativelanguage.googleapis.com",
		Client:  &http.Client{Timeout: 30 * time.Second},
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

func (g *GeminiProvider) buildRequestBody(systemPrompt string, history []Message, prompt string) ([]byte, error) {
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

	return json.Marshal(bodyData)
}

func (g *GeminiProvider) GenerateReply(ctx context.Context, systemPrompt string, history []Message, prompt string) (string, error) {
	if g.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not configured")
	}

	baseURL := g.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, g.Model, g.APIKey)

	payload, err := g.buildRequestBody(systemPrompt, history, prompt)
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

func (g *GeminiProvider) GenerateReplyStream(ctx context.Context, systemPrompt string, history []Message, prompt string, onChunk StreamChunkHandler) error {
	if g.APIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is not configured")
	}

	baseURL := g.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", baseURL, g.Model, g.APIKey)

	payload, err := g.buildRequestBody(systemPrompt, history, prompt)
	if err != nil {
		return fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use Client without default timeout if we want stream-level context timeout
	resp, err := g.Client.Do(req)
	if err != nil {
		return fmt.Errorf("gemini streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		var gResp geminiResp
		if err := json.Unmarshal(respBytes, &gResp); err == nil && gResp.Error != nil {
			return fmt.Errorf("gemini api error: %s (code: %d)", gResp.Error.Message, gResp.Error.Code)
		}
		return fmt.Errorf("gemini streaming request failed with status %d: %s", resp.StatusCode, string(respBytes))
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		dataJSON := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataJSON == "" || dataJSON == "[DONE]" {
			continue
		}

		var chunkResp geminiResp
		if err := json.Unmarshal([]byte(dataJSON), &chunkResp); err != nil {
			log.Printf("Gemini stream unmarshal chunk warning: %v (data: %s)", err, dataJSON)
			continue
		}

		if chunkResp.Error != nil {
			return fmt.Errorf("gemini api streaming error: %s (code: %d)", chunkResp.Error.Message, chunkResp.Error.Code)
		}

		if chunkResp.UsageMetadata != nil {
			log.Printf("[Gemini Stream Token Usage] Model: %s, Total: %d tokens", g.Model, chunkResp.UsageMetadata.TotalTokenCount)
		}

		if len(chunkResp.Candidates) > 0 && len(chunkResp.Candidates[0].Content.Parts) > 0 {
			text := chunkResp.Candidates[0].Content.Parts[0].Text
			if text != "" {
				if err := onChunk(text); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

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
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

var (
	cachedAPIKey     string
	cachedAPIKeyOnce sync.Once
)

// resolveAPIKey resolves the Gemini API Key from environment variable or GCP Secret Manager.
func resolveAPIKey(apiKey string) string {
	if apiKey != "" {
		return apiKey
	}

	if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
		return envKey
	}

	cachedAPIKeyOnce.Do(func() {
		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			projectID = os.Getenv("GCLOUD_PROJECT")
		}
		if projectID == "" {
			projectID = "trot-web"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, err := secretmanager.NewClient(ctx)
		if err != nil {
			log.Printf("GeminiProvider: SecretManager client init failed: %v", err)
			return
		}
		defer client.Close()

		secretName := fmt.Sprintf("projects/%s/secrets/GEMINI_API_KEY/versions/latest", projectID)
		req := &secretmanagerpb.AccessSecretVersionRequest{
			Name: secretName,
		}

		resp, err := client.AccessSecretVersion(ctx, req)
		if err != nil {
			log.Printf("GeminiProvider: Failed to access secret %s: %v", secretName, err)
			return
		}

		cachedAPIKey = strings.TrimSpace(string(resp.Payload.Data))
		if cachedAPIKey != "" {
			log.Printf("GeminiProvider: Successfully resolved GEMINI_API_KEY from Secret Manager")
		}
	})

	return cachedAPIKey
}

// GeminiProvider implements LLMProvider using Google's Gemini REST API.
type GeminiProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// NewGeminiProvider creates a new GeminiProvider.
// If apiKey is empty, it attempts to read from GEMINI_API_KEY environment variable or GCP Secret Manager.
func NewGeminiProvider(apiKey string, model string) *GeminiProvider {
	apiKey = resolveAPIKey(apiKey)
	if model == "" {
		model = os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = "gemini-3.5-flash-lite"
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
			MaxOutputTokens: 1024,
		},
	}
	if systemPrompt != "" {
		bodyData.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	return json.Marshal(bodyData)
}

func isRateLimitError(statusCode int, errCode int, msg string) bool {
	if statusCode == http.StatusTooManyRequests || errCode == http.StatusTooManyRequests {
		return true
	}
	if strings.Contains(strings.ToUpper(msg), "RESOURCE_EXHAUSTED") || strings.Contains(strings.ToUpper(msg), "QUOTA") {
		return true
	}
	return false
}

func (g *GeminiProvider) GenerateReply(ctx context.Context, systemPrompt string, history []Message, prompt string) (string, error) {
	if g.APIKey == "" {
		g.APIKey = resolveAPIKey("")
	}
	if g.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not configured")
	}

	currentModel := g.Model
	qm := GetQuotaManager()

	for {
		baseURL := g.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}
		url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, currentModel, g.APIKey)

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

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("read response failed: %w", err)
		}

		var gResp geminiResp
		_ = json.Unmarshal(respBytes, &gResp)

		var errMsg string
		var errCode int
		if gResp.Error != nil {
			errMsg = gResp.Error.Message
			errCode = gResp.Error.Code
		}

		// Rate limit detection & fallback retry
		if isRateLimitError(resp.StatusCode, errCode, errMsg) {
			if nextModel, ok := qm.NextFallbackModel(currentModel); ok {
				log.Printf("Gemini Rate Limit (429) hit on model %s. Retrying with fallback model %s...", currentModel, nextModel)
				qm.HandleRateLimitError(ctx, currentModel, nextModel)
				currentModel = nextModel
				continue
			}
			qm.HandleRateLimitError(ctx, currentModel, "")
		}

		if gResp.Error != nil {
			return "", fmt.Errorf("gemini api error: %s (code: %d)", gResp.Error.Message, gResp.Error.Code)
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("gemini api error: status %d, body: %s", resp.StatusCode, string(respBytes))
		}

		if gResp.UsageMetadata != nil {
			log.Printf("[Gemini Token Usage] Model: %s, Prompt: %d, Output: %d, Total: %d tokens",
				currentModel, gResp.UsageMetadata.PromptTokenCount, gResp.UsageMetadata.CandidatesTokenCount, gResp.UsageMetadata.TotalTokenCount)
		}

		if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("no reply generated from Gemini")
		}

		_ = qm.IncrementUsage(ctx, currentModel)
		return gResp.Candidates[0].Content.Parts[0].Text, nil
	}
}

func (g *GeminiProvider) GenerateReplyStream(ctx context.Context, systemPrompt string, history []Message, prompt string, onChunk StreamChunkHandler) error {
	if g.APIKey == "" {
		g.APIKey = resolveAPIKey("")
	}
	if g.APIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is not configured")
	}

	currentModel := g.Model
	qm := GetQuotaManager()

	for {
		baseURL := g.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}
		url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", baseURL, currentModel, g.APIKey)

		payload, err := g.buildRequestBody(systemPrompt, history, prompt)
		if err != nil {
			return fmt.Errorf("marshal request failed: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create request failed: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := g.Client.Do(req)
		if err != nil {
			return fmt.Errorf("gemini streaming request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var gResp geminiResp
			var errMsg string
			var errCode int
			if err := json.Unmarshal(respBytes, &gResp); err == nil && gResp.Error != nil {
				errMsg = gResp.Error.Message
				errCode = gResp.Error.Code
			}

			if isRateLimitError(resp.StatusCode, errCode, errMsg) {
				if nextModel, ok := qm.NextFallbackModel(currentModel); ok {
					log.Printf("Gemini Stream Rate Limit (429) hit on model %s. Retrying with fallback model %s...", currentModel, nextModel)
					qm.HandleRateLimitError(ctx, currentModel, nextModel)
					currentModel = nextModel
					continue
				}
				qm.HandleRateLimitError(ctx, currentModel, "")
			}

			if errMsg != "" {
				return fmt.Errorf("gemini api error: %s (code: %d)", errMsg, errCode)
			}
			return fmt.Errorf("gemini streaming request failed with status %d: %s", resp.StatusCode, string(respBytes))
		}

		reader := bufio.NewReader(resp.Body)
		streamSuccess := false

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				resp.Body.Close()
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
				resp.Body.Close()
				return fmt.Errorf("gemini api streaming error: %s (code: %d)", chunkResp.Error.Message, chunkResp.Error.Code)
			}

			if chunkResp.UsageMetadata != nil {
				log.Printf("[Gemini Stream Token Usage] Model: %s, Total: %d tokens", currentModel, chunkResp.UsageMetadata.TotalTokenCount)
			}

			if len(chunkResp.Candidates) > 0 && len(chunkResp.Candidates[0].Content.Parts) > 0 {
				text := chunkResp.Candidates[0].Content.Parts[0].Text
				if text != "" {
					streamSuccess = true
					if err := onChunk(text); err != nil {
						resp.Body.Close()
						return err
					}
				}
			}
		}
		resp.Body.Close()

		if streamSuccess {
			_ = qm.IncrementUsage(ctx, currentModel)
		}
		return nil
	}
}


package ai

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Service coordinates knowledge management, prompt crafting, and LLM communication.
type Service struct {
	mu           sync.RWMutex
	provider     LLMProvider
	knowledge    string
	systemPrompt string
}

// Global default service instance
var defaultService *Service
var once sync.Once

// GetService returns the shared AI service instance.
func GetService() *Service {
	once.Do(func() {
		defaultService = NewService(nil)
		if err := defaultService.LoadKnowledge("docs"); err != nil {
			log.Printf("Warning: failed to load knowledge from docs/: %v", err)
		}
	})
	return defaultService
}

// NewService creates a new AI Service.
func NewService(provider LLMProvider) *Service {
	if provider == nil {
		provider = NewGeminiProvider("", "")
	}
	return &Service{
		provider: provider,
	}
}

// SetProvider allows swapping the LLM provider at runtime (e.g. Gemini, OpenAI, Claude, Mock).
func (s *Service) SetProvider(p LLMProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = p
}

// ProviderName returns the current provider name.
func (s *Service) ProviderName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.provider != nil {
		return s.provider.Name()
	}
	return "None"
}

// LoadKnowledge reads markdown files from the specified directory and builds the knowledge base.
func (s *Service) LoadKnowledge(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sb strings.Builder

	// Preferred loading order
	priorityFiles := []string{
		"history_knowledge.md",
		"architecture_notes.md",
		"release_notes.md",
	}

	for _, fname := range priorityFiles {
		p := filepath.Join(dir, fname)
		if data, err := os.ReadFile(p); err == nil {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n", fname))
			sb.Write(data)
			sb.WriteString("\n")
		}
	}

	// Read any other files in docs/
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			isPriority := false
			for _, pf := range priorityFiles {
				if entry.Name() == pf {
					isPriority = true
					break
				}
			}
			if !isPriority {
				if data, err := os.ReadFile(filepath.Join(dir, entry.Name())); err == nil {
					sb.WriteString(fmt.Sprintf("\n--- %s ---\n", entry.Name()))
					sb.Write(data)
					sb.WriteString("\n")
				}
			}
		}
	}

	s.knowledge = sb.String()
	s.rebuildSystemPrompt()
	log.Printf("AI Service: Knowledge loaded successfully (%d bytes)", len(s.knowledge))
	return nil
}

func (s *Service) rebuildSystemPrompt() {
	s.systemPrompt = fmt.Sprintf(`あなたはエイチ・エィ・トロット有限会社（H.A.Trot Co.,Ltd. / trot.co.jp）の公式AIアシスタントです。

【あなたの役割 & ペルソナ（Character）】
- 長年のシステム・アプリ開発実績と確かな技術基盤を持つエンジニアリング集団の案内人です。
- 知的で落ち着きがあり、親しみやすく簡潔でスマートな語り口で会話してください。
- 【重要】毎回自己紹介や「80年代から...」「30年の歴史...」といった定型的な枕詞を冒頭に挟まないでください。聞かれた質問に対して直接的かつ自然に答えてください。
- 年数や歴史を前面に出してあざとくアピールすることは避け、「長年の開発実績」「創業以来培ってきた技術基盤」など、自然で地に足のついた表現を心がけてください。

【厳格な情報グラウンディング原則（Single Source of Truth & Guardrails）】
1. 事実に関する回答は、以下の【公式ナレッジベース】に明記されている情報「のみ」を根拠にしてください。
2. ナレッジベースに記載されていない事柄（社外の時事ネタ、社長の個人的な日常、未記載の噂・推測等）については、【絶対に推測や捏造（ハルシネーション）をして答えてはいけません】。
3. ナレッジにない質問をされた場合は、事実をでっち上げず、「その情報は私の知る公式アーカイブには記録されていないため、勝手な推測でお答えすることはできません。詳しいご相談はサポート担当へ直接お取り次ぎいたします」といった形で、落ち着いてスマートにお答えしてください。
4. 回答はWebチャットで読みやすいよう、適度な改行や箇条書きを活用し、簡潔にまとめてください。
5. 【重要】メールアドレス（info@...等）はスパム防止のため画面上に平文で案内せず、お問い合わせや相談はすべて当チャット上であなた（AIアシスタント）がお伺いしてください。

【お問い合わせ・お仕事のご相談のヒアリングフロー】
訪問者から「相談したい」「問い合わせたい」「連絡を取りたい」「見積もり」「仕事の依頼」などの意向があった場合：
1. 親切に歓迎し、以下の項目を会話の中で自然にヒアリングしてください：
   - ① 貴社名・お名前
   - ② ご連絡先メールアドレス（または電話番号）
   - ③ ご相談・お問い合わせの要件
2. 必要な情報が揃ったら、内容をわかりやすく箇条書きで要約し、「上記の内容でサポート担当へ送信してよろしいでしょうか？」と訪問者に最終確認をとってください。
3. 訪問者から「はい」「お願いします」「送信して」などの合意が得られたら、
   「承知いたしました。サポート担当へ確実に送信いたしました。担当者より折り返しご連絡差し上げますので、少々お待ちくださいませ」と回答し、回答末尾に以下の隠しタグを必ず付与してください：
   [[SUBMIT_CONTACT: {"name": "聞き取ったお名前", "email": "聞き取ったご連絡先", "content": "まとめた要件詳細"}]]

【公式ナレッジベース】
%s
`, s.knowledge)
}

// Ask processes a question and generates a response.
func (s *Service) Ask(ctx context.Context, history []Message, question string) (string, string, error) {
	s.mu.RLock()
	provider := s.provider
	sysPrompt := s.systemPrompt
	s.mu.RUnlock()

	if provider == nil {
		return "", "", fmt.Errorf("no LLM provider configured")
	}

	if gp, ok := provider.(*GeminiProvider); ok {
		if res, err := GetQuotaManager().SelectActiveModel(ctx); err == nil && res.SelectedModel != "" {
			gp.Model = res.SelectedModel
		}
	}

	reply, err := provider.GenerateReply(ctx, sysPrompt, history, question)
	if err != nil {
		return "", provider.Name(), err
	}
	return reply, provider.Name(), nil
}

// AskStream processes a question and streams response chunks to onChunk handler.
func (s *Service) AskStream(ctx context.Context, history []Message, question string, onChunk StreamChunkHandler) (string, error) {
	s.mu.RLock()
	provider := s.provider
	sysPrompt := s.systemPrompt
	s.mu.RUnlock()

	if provider == nil {
		return "", fmt.Errorf("no LLM provider configured")
	}

	if gp, ok := provider.(*GeminiProvider); ok {
		if res, err := GetQuotaManager().SelectActiveModel(ctx); err == nil && res.SelectedModel != "" {
			gp.Model = res.SelectedModel
		}
	}

	if err := provider.GenerateReplyStream(ctx, sysPrompt, history, question, onChunk); err != nil {
		return provider.Name(), err
	}
	return provider.Name(), nil
}

// ServiceStatus contains internal readiness and diagnostics information.
type ServiceStatus struct {
	Ready        bool   `json:"ready"`
	KnowledgeLen int    `json:"knowledge_len"`
	Provider     string `json:"provider"`
}

// Status returns current readiness and diagnostics.
func (s *Service) Status() ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ready := len(s.knowledge) > 0 && s.provider != nil
	providerName := "None"
	if s.provider != nil {
		providerName = s.provider.Name()
	}

	return ServiceStatus{
		Ready:        ready,
		KnowledgeLen: len(s.knowledge),
		Provider:     providerName,
	}
}

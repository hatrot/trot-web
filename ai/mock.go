package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MockProvider provides mock responses for offline testing and verification.
type MockProvider struct{}

func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Name() string {
	return "Mock (Offline Demo)"
}

func (m *MockProvider) GenerateReply(ctx context.Context, systemPrompt string, history []Message, prompt string) (string, error) {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "歴史") || strings.Contains(lower, "沿革") || strings.Contains(lower, "創業"):
		return "おっと、トロットの30年史に興味をお持ちですか！\n\n1984年のアセンブラゲーム開発に始まり、1993年の創業、1995年のクラスC IPアドレス取得・EC黎明期、そして携帯基地局制御やGoogle Cloud、最新の生成AIに至るまで、常に最先端の技術の荒波を泳ぎ続けています。\n\nまさに『技術の生き字引』と呼んでいただいても構いませんよ！", nil

	case strings.Contains(lower, "技術") || strings.Contains(lower, "スタック") || strings.Contains(lower, "開発"):
		return "ふふっ、技術の話になると熱が入ってしまいますね。\n\n私たちは低レイヤーのアセンブラ・C言語から、Go言語、Google App Engine (GAE) によるゼロスケール低コストインフラ、そして現代の生成AIソリューションまで幅広く得意としています。\n\n『軽くて速くて壊れない』、それが私たちの信条です！", nil

	case strings.Contains(lower, "nack5") || strings.Contains(lower, "プロダクト") || strings.Contains(lower, "raditas"):
		return "自慢のプロダクトですね！\n\nFM NACK5様などで活用された『NACK5TOUCH』や『NACK5VOICE』、そしてラジオ番組連動プラットフォーム『RADITAS』などを手がけました。生放送のリスナー熱量をリアルタイムにスタジオへ届ける仕組みです。\n\nリアルタイム同期の興奮はいつの時代も色褪せません！", nil

	default:
		return fmt.Sprintf("なるほど、「%s」ですね！\n\nあいにく私のトロット公式アーカイブにはまだ記載のない未知の領域です。勝手な憶測を語って社内コードレビューで怒られるわけにはいかないので（笑）、詳細なご相談やお問い合わせはぜひ [info@trot.co.jp](mailto:info@trot.co.jp) までお気軽にどうぞ！", prompt), nil
	}
}

func (m *MockProvider) GenerateReplyStream(ctx context.Context, systemPrompt string, history []Message, prompt string, onChunk StreamChunkHandler) error {
	fullText, err := m.GenerateReply(ctx, systemPrompt, history, prompt)
	if err != nil {
		return err
	}

	// Stream characters/runes in small batches to simulate typing
	runes := []rune(fullText)
	chunkSize := 3
	for i := 0; i < len(runes); i += chunkSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunk := string(runes[i:end])
		if err := onChunk(chunk); err != nil {
			return err
		}

		time.Sleep(15 * time.Millisecond)
	}

	return nil
}

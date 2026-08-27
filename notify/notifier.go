package notify

import (
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

// ContactMessage represents a user contact or inquiry payload.
type ContactMessage struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

// Notifier defines a common interface for dispatching notifications.
type Notifier interface {
	Send(ctx context.Context, msg ContactMessage) error
	Name() string
}

// CompositeNotifier dispatches to all configured channels.
type CompositeNotifier struct {
	notifiers []Notifier
}

// NewNotifier initializes a unified notification dispatcher based on environment variables.
func NewNotifier() Notifier {
	var list []Notifier

	// 1. Slack Webhook Notifier
	if slackURL := os.Getenv("SLACK_WEBHOOK_URL"); slackURL != "" {
		list = append(list, NewSlackNotifier(slackURL))
	}

	// 2. SendGrid Email Notifier
	if sgKey := os.Getenv("SENDGRID_API_KEY"); sgKey != "" {
		from := os.Getenv("CONTACT_FROM_EMAIL")
		to := os.Getenv("CONTACT_NOTIFY_EMAIL")
		if to != "" {
			list = append(list, NewSendGridNotifier(sgKey, from, to))
		}
	}

	// 3. Fallback Logger if no channels are configured
	if len(list) == 0 {
		list = append(list, NewLogNotifier())
	}

	return &CompositeNotifier{notifiers: list}
}

// Send broadcasts the message across all active notifiers.
func (c *CompositeNotifier) Send(ctx context.Context, msg ContactMessage) error {
	var errs []string
	for _, n := range c.notifiers {
		if err := n.Send(ctx, msg); err != nil {
			log.Printf("Notifier [%s] error: %v", n.Name(), err)
			errs = append(errs, fmt.Sprintf("%s: %v", n.Name(), err))
		} else {
			log.Printf("Notifier [%s] successfully delivered contact inquiry", n.Name())
		}
	}
	if len(errs) > 0 && len(errs) == len(c.notifiers) {
		return fmt.Errorf("all notifications failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (c *CompositeNotifier) Name() string {
	var names []string
	for _, n := range c.notifiers {
		names = append(names, n.Name())
	}
	return strings.Join(names, "+")
}

// ==================== Slack Notifier ====================

type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SlackNotifier) Name() string {
	return "Slack"
}

func (s *SlackNotifier) Send(ctx context.Context, msg ContactMessage) error {
	loc := time.FixedZone("Asia/Tokyo", 9*60*60)
	nowStr := time.Now().In(loc).Format("2006/01/02 15:04:05")

	emailStr := msg.Email
	if emailStr == "" {
		emailStr = "（未記入）"
	}
	sourceStr := msg.Source
	if sourceStr == "" {
		sourceStr = "AIアシスタント (Web Q&A)"
	}

	text := fmt.Sprintf("📩 *【新規お問い合わせ】%s*\n*お名前 / 貴社名:* %s\n*ご連絡先:* %s\n*受付日時:* %s\n*流入経路:* %s\n\n*📋 ご相談・要件メモ:*\n```\n%s\n```",
		msg.Name, msg.Name, emailStr, nowStr, sourceStr, msg.Content)

	payload := map[string]interface{}{
		"text": text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook error: status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ==================== SendGrid Notifier ====================

type SendGridNotifier struct {
	apiKey    string
	fromEmail string
	toEmail   string
	apiURL    string
	client    *http.Client
}

func NewSendGridNotifier(apiKey, fromEmail, toEmail string) *SendGridNotifier {
	if fromEmail == "" {
		fromEmail = "kato@trot.co.jp"
	}
	return &SendGridNotifier{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		toEmail:   toEmail,
		apiURL:    "https://api.sendgrid.com/v3/mail/send",
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (sg *SendGridNotifier) Name() string {
	return "SendGrid"
}

func (sg *SendGridNotifier) Send(ctx context.Context, msg ContactMessage) error {
	loc := time.FixedZone("Asia/Tokyo", 9*60*60)
	nowStr := time.Now().In(loc).Format("2006/01/02 15:04:05")

	emailStr := msg.Email
	if emailStr == "" {
		emailStr = "（未記入）"
	}

	subject := fmt.Sprintf("【Web問合せ】%s 様よりご相談（AIアシスタント受付）", msg.Name)
	bodyContent := fmt.Sprintf(`trot.co.jp の AIアシスタント経由で新規お問い合わせを受付いたしました。

■ お名前 / 貴社名:
%s

■ ご連絡先メールアドレス:
%s

■ 受付日時:
%s

■ ご相談・お問い合わせ内容:
--------------------------------------------------
%s
--------------------------------------------------

※ 本メールは Web サイトの AI アシスタント対話システムより自動転送されています。
`, msg.Name, emailStr, nowStr, msg.Content)

	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]string{
					{"email": sg.toEmail},
				},
			},
		},
		"from": map[string]string{
			"email": sg.fromEmail,
			"name":  "H.A.Trot AI Assistant",
		},
		"subject": subject,
		"content": []map[string]string{
			{
				"type":  "text/plain",
				"value": bodyContent,
			},
		},
	}

	if msg.Email != "" && strings.Contains(msg.Email, "@") {
		payload["reply_to"] = map[string]string{
			"email": msg.Email,
			"name":  msg.Name,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	apiEndpoint := sg.apiURL
	if apiEndpoint == "" {
		apiEndpoint = "https://api.sendgrid.com/v3/mail/send"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", apiEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+sg.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := sg.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendgrid error: status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ==================== Log Notifier (Fallback) ====================

type LogNotifier struct{}

func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}

func (l *LogNotifier) Name() string {
	return "Log"
}

func (l *LogNotifier) Send(ctx context.Context, msg ContactMessage) error {
	log.Printf("[Contact Inquiry Log] Name: %s, Email: %s, Source: %s, Content: %s",
		msg.Name, msg.Email, msg.Source, msg.Content)
	return nil
}

// SendSystemAlert sends a system-level alert (e.g. AI Quota warning / Model fallback) to Slack or Log.
func SendSystemAlert(ctx context.Context, title string, details string) error {
	loc := time.FixedZone("Asia/Tokyo", 9*60*60)
	nowStr := time.Now().In(loc).Format("2006/01/02 15:04:05")

	slackURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackURL == "" {
		log.Printf("[System Alert Log] %s | %s | %s", title, nowStr, details)
		return nil
	}

	text := fmt.Sprintf("%s\n*日時:* %s (JST)\n%s",
		title, nowStr, details)

	payload := map[string]interface{}{
		"text": text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", slackURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("SendSystemAlert Slack post error: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack alert error: status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}


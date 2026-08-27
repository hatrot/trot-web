package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/hatrot/trot-web/notify"
)

// ModelTier defines a single model's quota in the fallback chain.
type ModelTier struct {
	ModelName  string `json:"model_name"`
	DailyLimit int    `json:"daily_limit"`
}

// DefaultModelChain defines the standard priority chain for Gemini models.
// Default: 3.5 Lite -> 3.1 Lite.
// At 90% (450/500), 3.5 Lite switches to 3.1 Lite with Slack notification.
// When on the final model, 80% triggers a warning alert, and 90% triggers a critical alert.
var DefaultModelChain = []ModelTier{
	{
		ModelName:  "gemini-3.5-flash-lite",
		DailyLimit: 500,
	},
	{
		ModelName:  "gemini-3.1-flash-lite",
		DailyLimit: 500,
	},
}

// DailyUsageEntity represents the persistent record stored in Cloud Datastore.
type DailyUsageEntity struct {
	Date         string    `datastore:"date"`
	CountsJSON   string    `datastore:"counts_json,noindex"`
	NotifiedJSON string    `datastore:"notified_json,noindex"`
	UpdatedAt    time.Time `datastore:"updated_at"`
}

// QuotaStore abstracts the persistence layer for quota counting (Datastore vs In-Memory).
type QuotaStore interface {
	GetUsage(ctx context.Context, dateStr string) (counts map[string]int, notified map[string]bool, err error)
	Increment(ctx context.Context, dateStr string, modelName string) error
	SetNotified(ctx context.Context, dateStr string, notifKey string) (wasSet bool, err error)
}

// QuotaManager coordinates model selection, usage tracking, and alerts.
type QuotaManager struct {
	store     QuotaStore
	chain     []ModelTier
	nowFunc   func() time.Time
	alertFunc func(ctx context.Context, title string, details string) error
	mu        sync.Mutex
}

var (
	defaultManager *QuotaManager
	managerOnce    sync.Once
)

// GetQuotaManager returns the singleton QuotaManager instance.
func GetQuotaManager() *QuotaManager {
	managerOnce.Do(func() {
		defaultManager = NewQuotaManager(nil, DefaultModelChain)
	})
	return defaultManager
}

// NewQuotaManager creates a new QuotaManager with given store and model chain.
func NewQuotaManager(store QuotaStore, chain []ModelTier) *QuotaManager {
	if len(chain) == 0 {
		chain = DefaultModelChain
	}
	if store == nil {
		store = initDefaultStore()
	}
	return &QuotaManager{
		store:     store,
		chain:     chain,
		nowFunc:   time.Now,
		alertFunc: notify.SendSystemAlert,
	}
}

func initDefaultStore() QuotaStore {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GCLOUD_PROJECT")
	}
	if projectID == "" {
		projectID = "trot-web"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dsClient, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		log.Printf("QuotaManager: Datastore init failed (falling back to MemoryStore): %v", err)
		return NewMemoryStore()
	}

	return &DatastoreStore{
		client: dsClient,
	}
}

func (qm *QuotaManager) todayStr() string {
	loc := time.FixedZone("Asia/Tokyo", 9*60*60)
	return qm.nowFunc().In(loc).Format("2006-01-02")
}

// ModelSelectionResult contains the selected model and decision details.
type ModelSelectionResult struct {
	SelectedModel string
	TierIndex     int
	Count         int
	DailyLimit    int
	IsFallback    bool
}

// SelectActiveModel evaluates the chain in priority order and selects the best active model.
// Rules:
// 1. 80% (8割越え):
//    - If NO next model exists (final tier or single model): fires an 80% warning alert.
// 2. 90% (9割越え):
//    - If next model exists: fires a switch alert and switches to the next model.
//    - If NO next model exists: fires a 90% critical alert and stays on this model.
func (qm *QuotaManager) SelectActiveModel(ctx context.Context) (ModelSelectionResult, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	dateStr := qm.todayStr()
	counts, notified, err := qm.store.GetUsage(ctx, dateStr)
	if err != nil {
		log.Printf("QuotaManager GetUsage error: %v (using primary model)", err)
		return ModelSelectionResult{
			SelectedModel: qm.chain[0].ModelName,
			TierIndex:     0,
		}, nil
	}

	chainLen := len(qm.chain)
	for i, tier := range qm.chain {
		currentCount := counts[tier.ModelName]
		limit := tier.DailyLimit
		threshold80 := int(float64(limit) * 0.80)
		threshold90 := int(float64(limit) * 0.90)
		isLast := (i == chainLen-1)

		// 1. Check 80% threshold (only when NO next model exists)
		if isLast && currentCount >= threshold80 {
			notif80Key := fmt.Sprintf("%s_80", tier.ModelName)
			if !notified[notif80Key] {
				if wasSet, _ := qm.store.SetNotified(ctx, dateStr, notif80Key); wasSet {
					title := "⚠️ *【AI利用枠 80% 警告】*"
					details := fmt.Sprintf(
						"*状況:* 最終モデル `%s` が %d/%d 回 (80%%) に到達\n*注意:* 次の待機モデルはありません。上限到達にご注意ください",
						tier.ModelName, currentCount, limit,
					)
					go func(t, d string) {
						_ = qm.alertFunc(context.Background(), t, d)
					}(title, details)
				}
			}
		}

		// 2. Check if under 90% threshold -> Use this model
		if currentCount < threshold90 {
			return ModelSelectionResult{
				SelectedModel: tier.ModelName,
				TierIndex:     i,
				Count:         currentCount,
				DailyLimit:    limit,
				IsFallback:    i > 0,
			}, nil
		}

		// 3. At or above 90% threshold
		notif90Key := fmt.Sprintf("%s_90", tier.ModelName)
		if !notified[notif90Key] {
			if wasSet, _ := qm.store.SetNotified(ctx, dateStr, notif90Key); wasSet {
				if !isLast {
					nextTier := qm.chain[i+1]
					title := "⚠️ *【AIモデル自動切替】*"
					details := fmt.Sprintf(
						"*状況:* `%s` が %d/%d 回 (90%%) に到達\n*対応:* 次の待機モデル `%s` (上限: %d回) へ切り替えました",
						tier.ModelName, currentCount, limit, nextTier.ModelName, nextTier.DailyLimit,
					)
					go func(t, d string) {
						_ = qm.alertFunc(context.Background(), t, d)
					}(title, details)
				} else {
					title := "🚨 *【AI利用枠 90% 緊急警告】*"
					details := fmt.Sprintf(
						"*状況:* 最終モデル `%s` が %d/%d 回 (90%%) に到達\n*注意:* 本日の無料枠は残り %d 回です",
						tier.ModelName, currentCount, limit, limit-currentCount,
					)
					go func(t, d string) {
						_ = qm.alertFunc(context.Background(), t, d)
					}(title, details)
				}
			}
		}

		// If there is a next model, proceed to evaluate next tier
		if !isLast {
			continue
		}

		// If this is the last model, use it until hard limit
		return ModelSelectionResult{
			SelectedModel: tier.ModelName,
			TierIndex:     i,
			Count:         currentCount,
			DailyLimit:    limit,
			IsFallback:    i > 0,
		}, nil
	}


	// Fallback to first model
	return ModelSelectionResult{
		SelectedModel: qm.chain[0].ModelName,
		TierIndex:     0,
	}, nil
}

// IncrementUsage records +1 for the specified model in today's usage using atomic transaction.
func (qm *QuotaManager) IncrementUsage(ctx context.Context, modelName string) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	dateStr := qm.todayStr()
	return qm.store.Increment(ctx, dateStr, modelName)
}

// NextFallbackModel returns the next model in the chain after failedModel.
func (qm *QuotaManager) NextFallbackModel(failedModel string) (string, bool) {
	for i, tier := range qm.chain {
		if tier.ModelName == failedModel && i+1 < len(qm.chain) {
			return qm.chain[i+1].ModelName, true
		}
	}
	return "", false
}

// HandleRateLimitError sends an alert when Google returns 429 RESOURCE_EXHAUSTED.
// It ensures that only the FIRST 429 error of the day per model sends an alert, avoiding spam.
func (qm *QuotaManager) HandleRateLimitError(ctx context.Context, failedModel string, nextModel string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	dateStr := qm.todayStr()
	notif429Key := fmt.Sprintf("%s_429", failedModel)
	wasSet, err := qm.store.SetNotified(ctx, dateStr, notif429Key)
	if err != nil {
		log.Printf("QuotaManager HandleRateLimitError SetNotified error: %v", err)
	}
	if !wasSet {
		// Already notified today for this model's 429 error
		return
	}

	title := "🚨 *【Gemini API クォータ制限 (429)】*"
	var details string
	if nextModel != "" {
		details = fmt.Sprintf("*状況:* `%s` で 429 エラーを検知\n*対応:* 次点モデル `%s` に緊急フェイルオーバーしました", failedModel, nextModel)
	} else {
		details = fmt.Sprintf("*状況:* `%s` で 429 エラーを検知\n*対応:* 利用可能な予備モデルがないため一時待機モードとなります", failedModel)
	}

	go func(t, d string) {
		_ = qm.alertFunc(context.Background(), t, d)
	}(title, details)
}

// ==================== Datastore Store Implementation ====================

type DatastoreStore struct {
	client *datastore.Client
}

func (ds *DatastoreStore) key(dateStr string) *datastore.Key {
	return datastore.NameKey("GeminiDailyUsage", dateStr, nil)
}

func (ds *DatastoreStore) GetUsage(ctx context.Context, dateStr string) (map[string]int, map[string]bool, error) {
	counts := make(map[string]int)
	notified := make(map[string]bool)

	if ds.client == nil {
		return counts, notified, nil
	}

	k := ds.key(dateStr)
	var ent DailyUsageEntity
	err := ds.client.Get(ctx, k, &ent)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return counts, notified, nil
		}
		return counts, notified, err
	}

	if ent.CountsJSON != "" {
		_ = json.Unmarshal([]byte(ent.CountsJSON), &counts)
	}
	if ent.NotifiedJSON != "" {
		_ = json.Unmarshal([]byte(ent.NotifiedJSON), &notified)
	}

	return counts, notified, nil
}

// Increment atomically increases a model's count using a Datastore transaction across multiple instances.
func (ds *DatastoreStore) Increment(ctx context.Context, dateStr string, modelName string) error {
	if ds.client == nil {
		return nil
	}

	k := ds.key(dateStr)
	_, err := ds.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var ent DailyUsageEntity
		counts := make(map[string]int)
		notified := make(map[string]bool)

		err := tx.Get(k, &ent)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}

		if ent.CountsJSON != "" {
			_ = json.Unmarshal([]byte(ent.CountsJSON), &counts)
		}
		if ent.NotifiedJSON != "" {
			_ = json.Unmarshal([]byte(ent.NotifiedJSON), &notified)
		}

		counts[modelName]++

		countsBytes, _ := json.Marshal(counts)
		notifiedBytes, _ := json.Marshal(notified)

		ent.Date = dateStr
		ent.CountsJSON = string(countsBytes)
		ent.NotifiedJSON = string(notifiedBytes)
		ent.UpdatedAt = time.Now()

		_, err = tx.Put(k, &ent)
		return err
	})
	return err
}

// SetNotified atomically marks a notification key as sent, returning true if this is the first notification.
func (ds *DatastoreStore) SetNotified(ctx context.Context, dateStr string, notifKey string) (bool, error) {
	if ds.client == nil {
		return false, nil
	}

	k := ds.key(dateStr)
	var wasSet bool
	_, err := ds.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var ent DailyUsageEntity
		counts := make(map[string]int)
		notified := make(map[string]bool)

		err := tx.Get(k, &ent)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}

		if ent.CountsJSON != "" {
			_ = json.Unmarshal([]byte(ent.CountsJSON), &counts)
		}
		if ent.NotifiedJSON != "" {
			_ = json.Unmarshal([]byte(ent.NotifiedJSON), &notified)
		}

		if notified[notifKey] {
			wasSet = false
			return nil
		}

		notified[notifKey] = true
		wasSet = true

		countsBytes, _ := json.Marshal(counts)
		notifiedBytes, _ := json.Marshal(notified)

		ent.Date = dateStr
		ent.CountsJSON = string(countsBytes)
		ent.NotifiedJSON = string(notifiedBytes)
		ent.UpdatedAt = time.Now()

		_, err = tx.Put(k, &ent)
		return err
	})
	return wasSet, err
}

// ==================== In-Memory Store (Local / Fallback) ====================

type MemoryStore struct {
	mu       sync.RWMutex
	counts   map[string]map[string]int
	notified map[string]map[string]bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		counts:   make(map[string]map[string]int),
		notified: make(map[string]map[string]bool),
	}
}

func (m *MemoryStore) GetUsage(ctx context.Context, dateStr string) (map[string]int, map[string]bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.counts[dateStr]
	if !ok {
		c = make(map[string]int)
	}
	copyC := make(map[string]int)
	for k, v := range c {
		copyC[k] = v
	}

	n, ok := m.notified[dateStr]
	if !ok {
		n = make(map[string]bool)
	}
	copyN := make(map[string]bool)
	for k, v := range n {
		copyN[k] = v
	}

	return copyC, copyN, nil
}

func (m *MemoryStore) Increment(ctx context.Context, dateStr string, modelName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.counts[dateStr]
	if !ok {
		c = make(map[string]int)
	}
	c[modelName]++
	m.counts[dateStr] = c
	return nil
}

func (m *MemoryStore) SetNotified(ctx context.Context, dateStr string, notifKey string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n, ok := m.notified[dateStr]
	if !ok {
		n = make(map[string]bool)
	}
	if n[notifKey] {
		return false, nil
	}

	n[notifKey] = true
	m.notified[dateStr] = n
	return true, nil
}



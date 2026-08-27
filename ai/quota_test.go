package ai

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockAlertRecorder struct {
	mu     sync.Mutex
	alerts []struct {
		Title   string
		Details string
	}
}

func (m *mockAlertRecorder) SendAlert(ctx context.Context, title, details string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, struct {
		Title   string
		Details string
	}{Title: title, Details: details})
	return nil
}

func (m *mockAlertRecorder) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.alerts)
}

func (m *mockAlertRecorder) LastAlert() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.alerts) == 0 {
		return "", ""
	}
	last := m.alerts[len(m.alerts)-1]
	return last.Title, last.Details
}

// ==================== 1. 単数モデル (チェーン長 = 1) の 8割警告 & 9割緊急通知 ====================

func TestQuotaManager_SingleModel_80_90_Rules(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	recorder := &mockAlertRecorder{}

	chain := []ModelTier{
		{
			ModelName:  "gemini-test-single",
			DailyLimit: 10, // 80% = 8回, 90% = 9回
		},
	}

	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	qm := NewQuotaManager(store, chain)
	qm.alertFunc = recorder.SendAlert
	qm.nowFunc = func() time.Time { return fixedTime }

	// 1. カウント 0 〜 7 回（8割未満）: 通知ゼロ
	for i := 0; i < 8; i++ {
		res, err := qm.SelectActiveModel(ctx)
		if err != nil {
			t.Fatalf("Step %d: unexpected error: %v", i, err)
		}
		if res.SelectedModel != "gemini-test-single" {
			t.Errorf("Step %d: expected gemini-test-single, got %s", i, res.SelectedModel)
		}
		_ = qm.IncrementUsage(ctx, "gemini-test-single")
	}

	if recorder.Count() != 0 {
		t.Errorf("Expected 0 alerts before count 8, got %d", recorder.Count())
	}

	// 2. カウント 8 回（80% 到達、次モデルなし）: 1回目の 80% 警告通知が飛ぶこと
	res, err := qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("At 80%%: unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-test-single" {
		t.Errorf("Expected gemini-test-single, got %s", res.SelectedModel)
	}
	_ = qm.IncrementUsage(ctx, "gemini-test-single") // count = 9

	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 1 {
		t.Fatalf("Expected 1 alert at 80%%, got %d", recorder.Count())
	}
	title, _ := recorder.LastAlert()
	if title == "" || !contains(title, "80%") {
		t.Errorf("Expected 80%% alert title, got: %s", title)
	}

	// 3. カウント 9 回（90% 到達、次モデルなし）: 2回目の 90% 緊急通知が飛ぶこと
	res, err = qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("At 90%%: unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-test-single" {
		t.Errorf("Expected gemini-test-single, got %s", res.SelectedModel)
	}
	_ = qm.IncrementUsage(ctx, "gemini-test-single") // count = 10

	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 2 {
		t.Fatalf("Expected 2 alerts at 90%%, got %d", recorder.Count())
	}
	title, _ = recorder.LastAlert()
	if title == "" || !contains(title, "90%") {
		t.Errorf("Expected 90%% alert title, got: %s", title)
	}

	// 4. カウント 10 回（100%）以降: 通知が重複しないこと
	for i := 0; i < 3; i++ {
		_, _ = qm.SelectActiveModel(ctx)
		_ = qm.IncrementUsage(ctx, "gemini-test-single")
	}
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 2 {
		t.Errorf("Duplicate alert detected! Expected count=2, got %d", recorder.Count())
	}
}

// ==================== 2. 複数モデルチェーン (3.5 Lite 90%切替 -> 3.1 Lite 80%/90%警告) ====================

func TestQuotaManager_MultiModelChain_80_90_Rules(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	recorder := &mockAlertRecorder{}

	chain := []ModelTier{
		{
			ModelName:  "gemini-3.5-flash-lite",
			DailyLimit: 500, // 90% = 450回で次へ切替
		},
		{
			ModelName:  "gemini-3.1-flash-lite",
			DailyLimit: 500, // 次なし -> 80%=400回で警告, 90%=450回で緊急警告
		},
	}

	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	qm := NewQuotaManager(store, chain)
	qm.alertFunc = recorder.SendAlert
	qm.nowFunc = func() time.Time { return fixedTime }

	// 1. 3.5 Lite が 400回 (80%) を超えても、次に 3.1 Lite があるのでアラートは飛ばない（90%まで継続）
	for i := 0; i < 449; i++ {
		_ = qm.IncrementUsage(ctx, "gemini-3.5-flash-lite")
	}
	res, err := qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-3.5-flash-lite" {
		t.Errorf("Expected 3.5-flash-lite before 90%%, got %s", res.SelectedModel)
	}
	if recorder.Count() != 0 {
		t.Errorf("Expected 0 alerts for primary model at 80%% (has next model), got %d", recorder.Count())
	}

	// 2. 3.5 Lite が 450回 (90%) 到達 -> 3.1 Lite へ自動切り替え ＆ 切替通知送信
	_ = qm.IncrementUsage(ctx, "gemini-3.5-flash-lite") // count = 450
	res, err = qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-3.1-flash-lite" || !res.IsFallback {
		t.Errorf("Expected switched to 3.1-flash-lite, got %s (fallback=%v)", res.SelectedModel, res.IsFallback)
	}
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 1 {
		t.Fatalf("Expected 1 alert for 90%% switch, got %d", recorder.Count())
	}

	// 3. サブモデル 3.1 Lite が 399回 -> まだアラートは 1回のまま
	for i := 0; i < 399; i++ {
		_ = qm.IncrementUsage(ctx, "gemini-3.1-flash-lite")
	}
	if recorder.Count() != 1 {
		t.Errorf("Expected 1 alert at 3.1 count 399, got %d", recorder.Count())
	}

	// 4. サブモデル 3.1 Lite が 400回 (80% 到達、最終モデル) -> 80% 警告通知 (計2通目)
	_ = qm.IncrementUsage(ctx, "gemini-3.1-flash-lite") // count = 400
	res, err = qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-3.1-flash-lite" {
		t.Errorf("Expected 3.1-flash-lite, got %s", res.SelectedModel)
	}
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 2 {
		t.Fatalf("Expected 2 alerts total (3.5 switch + 3.1 80%% warning), got %d", recorder.Count())
	}

	// 5. サブモデル 3.1 Lite が 450回 (90% 到達、最終モデル) -> 90% 緊急通知 (計3通目)
	for i := 400; i < 450; i++ {
		_ = qm.IncrementUsage(ctx, "gemini-3.1-flash-lite")
	}
	res, err = qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-3.1-flash-lite" {
		t.Errorf("Expected 3.1-flash-lite, got %s", res.SelectedModel)
	}
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 3 {
		t.Fatalf("Expected 3 alerts total (3.5 switch + 3.1 80%% + 3.1 90%%), got %d", recorder.Count())
	}
}

// ==================== 3. 日付跨ぎ（翌日リセット）のエッジケース ====================

func TestQuotaManager_DateRollOver(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	recorder := &mockAlertRecorder{}

	chain := []ModelTier{
		{
			ModelName:  "gemini-3.5-flash-lite",
			DailyLimit: 10,
		},
		{
			ModelName:  "gemini-3.1-flash-lite",
			DailyLimit: 10,
		},
	}

	// Day 1
	day1 := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	currentDay := day1
	qm := NewQuotaManager(store, chain)
	qm.alertFunc = recorder.SendAlert
	qm.nowFunc = func() time.Time { return currentDay }

	// Day 1 で 3.5 Lite を 9回 (90%到達) 消費 -> 3.1 Lite に切り替わり
	for i := 0; i < 9; i++ {
		_ = qm.IncrementUsage(ctx, "gemini-3.5-flash-lite")
	}
	res, _ := qm.SelectActiveModel(ctx)
	if res.SelectedModel != "gemini-3.1-flash-lite" {
		t.Errorf("Day 1: expected 3.1-flash-lite, got %s", res.SelectedModel)
	}
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 1 {
		t.Errorf("Day 1: expected 1 alert, got %d", recorder.Count())
	}

	// Day 2 (翌日) に移行 -> カウントと通知フラグが自動リセットされ、再び 3.5 Lite が選ばれること
	currentDay = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	res, err := qm.SelectActiveModel(ctx)
	if err != nil {
		t.Fatalf("Day 2: unexpected error: %v", err)
	}
	if res.SelectedModel != "gemini-3.5-flash-lite" || res.IsFallback {
		t.Errorf("Day 2: expected clean reset to 3.5-flash-lite, got %s (fallback=%v)", res.SelectedModel, res.IsFallback)
	}
}

// ==================== 4. 429 エラー時の NextFallbackModel と緊急アラート ====================

func TestQuotaManager_RateLimitFallback(t *testing.T) {
	recorder := &mockAlertRecorder{}
	chain := []ModelTier{
		{ModelName: "model-A", DailyLimit: 100},
		{ModelName: "model-B", DailyLimit: 100},
		{ModelName: "model-C", DailyLimit: 100},
	}
	qm := NewQuotaManager(NewMemoryStore(), chain)
	qm.alertFunc = recorder.SendAlert

	// 1. model-A failed -> next is model-B
	next, ok := qm.NextFallbackModel("model-A")
	if !ok || next != "model-B" {
		t.Errorf("Expected model-B, got %s (ok=%v)", next, ok)
	}

	// 2. model-B failed -> next is model-C
	next, ok = qm.NextFallbackModel("model-B")
	if !ok || next != "model-C" {
		t.Errorf("Expected model-C, got %s (ok=%v)", next, ok)
	}

	// 3. model-C failed -> no more models
	next, ok = qm.NextFallbackModel("model-C")
	if ok || next != "" {
		t.Errorf("Expected no next model, got %s (ok=%v)", next, ok)
	}

	// 4. HandleRateLimitError sends alert on 1st call, but suppresses on 2nd+ calls
	qm.HandleRateLimitError(context.Background(), "model-A", "model-B")
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 1 {
		t.Errorf("Expected 1 rate limit alert on first 429, got %d", recorder.Count())
	}

	// 2nd, 3rd 429 error on model-A on the same day -> suppressed!
	qm.HandleRateLimitError(context.Background(), "model-A", "model-B")
	qm.HandleRateLimitError(context.Background(), "model-A", "model-B")
	time.Sleep(50 * time.Millisecond)
	if recorder.Count() != 1 {
		t.Errorf("Expected still 1 alert (duplicate 429 suppressed), got %d", recorder.Count())
	}
}

// ==================== 5. 並行アクセス（Race Condition）テスト ====================

func TestQuotaManager_ConcurrencyRace(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	recorder := &mockAlertRecorder{}

	chain := []ModelTier{
		{ModelName: "concurrent-model-1", DailyLimit: 1000},
		{ModelName: "concurrent-model-2", DailyLimit: 1000},
	}

	qm := NewQuotaManager(store, chain)
	qm.alertFunc = recorder.SendAlert

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				res, err := qm.SelectActiveModel(ctx)
				if err == nil {
					_ = qm.IncrementUsage(ctx, res.SelectedModel)
				}
			}
		}()
	}

	wg.Wait()

	// 合計カウントが正確に workers * iterations (1,000) になっているか検証
	counts, _, err := store.GetUsage(ctx, qm.todayStr())
	if err != nil {
		t.Fatalf("Failed to get usage: %v", err)
	}

	total := counts["concurrent-model-1"] + counts["concurrent-model-2"]
	expected := workers * iterations
	if total != expected {
		t.Errorf("Concurrency count mismatch: expected exactly %d total calls, got %d (model1: %d, model2: %d)",
			expected, total, counts["concurrent-model-1"], counts["concurrent-model-2"])
	}
}

// ==================== 6. 複数インスタンス（マルチインスタンス）競合テスト ====================

func TestQuotaManager_MultiInstanceConcurrency(t *testing.T) {
	ctx := context.Background()
	sharedStore := NewMemoryStore()
	recorder := &mockAlertRecorder{}

	chain := []ModelTier{
		{ModelName: "multi-model-1", DailyLimit: 100},
		{ModelName: "multi-model-2", DailyLimit: 100},
	}

	// 3つの独立した App Engine インスタンスをシミュレート
	inst1 := NewQuotaManager(sharedStore, chain)
	inst1.alertFunc = recorder.SendAlert
	inst2 := NewQuotaManager(sharedStore, chain)
	inst2.alertFunc = recorder.SendAlert
	inst3 := NewQuotaManager(sharedStore, chain)
	inst3.alertFunc = recorder.SendAlert

	instances := []*QuotaManager{inst1, inst2, inst3}

	var wg sync.WaitGroup
	workers := 15
	perWorkerCalls := 20 // 15 * 20 = 300 calls total (multi-model-1 90 calls -> switch to multi-model-2)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		instIndex := w % len(instances)
		qm := instances[instIndex]

		go func(manager *QuotaManager) {
			defer wg.Done()
			for i := 0; i < perWorkerCalls; i++ {
				res, err := manager.SelectActiveModel(ctx)
				if err == nil {
					_ = manager.IncrementUsage(ctx, res.SelectedModel)
				}
			}
		}(qm)
	}

	wg.Wait()

	counts, _, err := sharedStore.GetUsage(ctx, inst1.todayStr())
	if err != nil {
		t.Fatalf("Failed to get shared store usage: %v", err)
	}

	total := counts["multi-model-1"] + counts["multi-model-2"]
	expected := workers * perWorkerCalls
	if total != expected {
		t.Errorf("Multi-instance total mismatch: expected %d, got %d (m1: %d, m2: %d)",
			expected, total, counts["multi-model-1"], counts["multi-model-2"])
	}

	// multi-model-1 が 90% (90回) 前後で multi-model-2 に切り替わっていることを確認
	if counts["multi-model-1"] < 90 {
		t.Errorf("Expected model-1 to reach ~90, got %d", counts["multi-model-1"])
	}
	if counts["multi-model-2"] == 0 {
		t.Errorf("Expected model-2 to receive overflow traffic, got 0")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

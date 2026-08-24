# trot-web

エイチ・エィ・トロット有限会社（**H.A.Trot Co.,Ltd.** / [https://trot.co.jp/](https://trot.co.jp/)）の公式 Web サイト公開用リポジトリです。  
Google Cloud (GAE Standard Go + Cloud Storage) によるゼロスケール・高耐久・低コスト運用アーキテクチャおよび Gemini 2.0 Flash 連携 AI Q&A を採用しています。

---

## 🏛️ ディレクトリ構成

```
trot-web/
  ├── ai/                   # 【AI Engine】Gemini 2.0 Flash REST クライアント & RAG サービス
  ├── docs/                 # 【Knowledge & Docs】会社沿革・プロダクト実績・アーキテクチャ設計・運用ナレッジ
  │    ├── history_knowledge.md     # 公式会社沿革 & プロダクトヒストリ
  │    ├── architecture_notes.md    # GFE / エッジキャッシュ / DDoS防御 / ゼロスケール解説
  │    ├── release_notes.md         # リリースノート
  │    └── troubleshooting_ogp.md   # OGP 最適化ドキュメント
  ├── www/                  # 【Web Contents】本番配信 HTML / CSS / 画像 / JS (フロストガラスUI & OGP)
  ├── app.yaml              # GAE Standard (Go 1.23 / F1 / min_instances: 0) 設定
  └── main.go               # GCS 連携配信 & /api/chat エンドポイント
```

---

## ⚡ システムアーキテクチャの特長

1. **Google Front End (GFE) による常時 DDoS 緩和**:
   - Google のグローバルネットワーク（L3/L4 DDoS 自動無効化）を初期状態で利用。
2. **ゼロスケール運用 (`min_instances: 0`)**:
   - アイドル時は 0 台、リクエスト着信時に Go が数 10ms で高速起動。常時無料枠内で運用。
3. **エッジキャッシュ × GCS ノーコード更新**:
   - 静的リソースは Google エッジキャッシュから配信。コンテンツ更新は GCS 同期のみで完了。
4. **Gemini 2.0 Flash による AI Q&A (RAG)**:
   - `docs/history_knowledge.md` を正本（Single Source of Truth）として回答するハルシネーション防止設計。

詳細なアーキテクチャ解説やシステム接続図は [docs/architecture_notes.md](docs/architecture_notes.md) をご覧ください。

---

## 🛠️ 開発環境セットアップ & ローカル実行

### 1. 依存パッケージの取得
```bash
go mod download
```

### 2. ローカル実行 (Gemini API 連携テスト)
```bash
# Gemini API Key を設定して起動
export GEMINI_API_KEY="your-gemini-api-key"
go run main.go
```
起動後、ブラウザで `http://localhost:8080` にアクセスしてください。

---

## 🚀 デプロイ手順 (Google Cloud)

### 1. Web 静的コンテンツ（HTML/CSS/画像/JS）の更新
```bash
gcloud storage rsync -r www gs://trot-web.appspot.com/www
```

### 2. Go プログラム本体・設定の更新
```bash
gcloud app deploy --version=master --project trot-web
```

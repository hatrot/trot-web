# OGP (Open Graph Protocol) 表示崩れの原因切り分け・1200x630px 規格化・実測検証レポート

## 1. 発生していた問題と現象

### 概要
従来、Webサイト (`https://trot.co.jp/`) の OGP メタタグ (`og:image`) には **`256 × 256 px` (1:1 正方形)** のロゴ画像単体が指定されていました。

この設定において、SNS やメッセージングアプリで URL がシェアされた際、プラットフォームごとに以下の表示ギャップ・崩れが発生していました。

| プラットフォームグループ | 表示挙動 | 発生していた問題 |
| :--- | :--- | :--- |
| **iMessage (Apple Rich Link) / Facebook** | 大判枠 (`1.91 : 1`) へ強制拡大 | 画像が巨大化・上部や下部が大きく切り抜かれ（トリミング崩れ）、カード全体を引き伸ばして崩れる。 |
| **𝕏 (Twitter) / Discord / LinkedIn** | コンパクトな小正方形サムネイル (`summary`) | カードの端に小さく正方形で表示される（大判カード表示にならない）。 |

---

## 2. 原因の切り分け（客観的エビデンス）

エミュレーター（**OpenGraph.xyz**）を Chrome DevTools Protocol (CDP: `localhost:9222`) 経由で操作し、客観的診断を実施しました。

### 診断パネル (Meta-tag inspector) の検出結果
* ❌ **`Image aspect ratio is wrong` (og:image)**
  * **エラー内容**: *`Detected 256x256 — most platforms expect 1.91:1 (1200x630). Card may be cropped or letterboxed.`*
  * **原因特定**: ほぼ全ての主要プラットフォームは **`1.91 : 1` (1200×630px)** の横長アスペクト比を標準仕様として設計されているため、正方形 `256×256` 画像をそのまま指定していることが表示崩れの根本原因であることを特定。

---

## 3. 修正方法と解決策

### 3.1 業界標準規格の OGP 専用アイキャッチ画像への変換
* **標準規格**: **`1200 × 630 px`** (アスペクト比 `1.91 : 1`)
* **画像ファイル**: `www/images/ogp_card.png`
* **デザイン仕様**:
  * Webサイト本体のヒーローセクション（有機CGオーブ＋すりガラス）の世界観を完全再現。
  * 立体感のある緑のシルクハットアイコン ＋ 重厚なセリフ体社名 (`H.A.Trot Co.,Ltd.`) ＋ メインキャッチコピー (`漠然とした想いから、システムの未来のカタチへ。`)

### 3.2 HTML メタタグの更新 (`www/index.html`)
`summary` (小サムネイル) から `summary_large_image` (大判全面カード) へ規格化。

```html
<!-- Open Graph / Facebook / SNS Card (OGP) -->
<meta property="og:type" content="website">
<meta property="og:url" content="https://trot.co.jp/">
<meta property="og:title" content="H.A.Trot Co.,Ltd. | Official Website">
<meta property="og:description" content="漠然とした想いから、システムの未来のカタチへ。横浜のシステム開発、モバイルソリューション、ラジオ連携基盤、AI技術支援。">
<meta property="og:image" content="https://trot.co.jp/images/ogp_card.png">

<!-- Twitter Card -->
<meta property="twitter:card" content="summary_large_image">
<meta property="twitter:url" content="https://trot.co.jp/">
<meta property="twitter:title" content="H.A.Trot Co.,Ltd. | Official Website">
<meta property="twitter:description" content="漠然とした想いから、システムの未来のカタチへ。横浜のシステム開発、モバイルソリューション、ラジオ連携基盤、AI技術支援。">
<meta property="twitter:image" content="https://trot.co.jp/images/ogp_card.png">
```

---

## 4. エミュレーターでの実測検証と本番リリース

### 4.1 本番サーバーへのデプロイ
* 本番バケット `gs://trot-web.appspot.com/www/` へ `ogp_card.png` および `index.html` を同期リリース。
* 配信確認: `https://trot.co.jp/images/ogp_card.png` (`HTTP/2 200 OK`)

### 4.2 OpenGraph エミュレーターでの最終実測判定
OpenGraph.xyz 上で `https://trot.co.jp/` を再読み込み (Rescan) して検証：

1. ❌ **アスペクト比エラー**: **`0` 件 (エラー完全消滅)**
2. **描画品質**: iMessage, Facebook, 𝕏, Discord, LinkedIn の全プラットフォームで、切れることなくカード全面へ**ノーカット・高精細の大判カード (`1.91 : 1`)** として描画されることを確認。

---

## 5. 今後・他のプロジェクトへの水平展開ガイドライン
* OGP 画像を指定する際は、単体ロゴ画像（正方形）ではなく、**必ず `1200 × 630 px` (`1.91 : 1`) の OGP 専用アイキャッチ画像を作成して指定する**。
* メタタグには `twitter:card="summary_large_image"` を併記する。

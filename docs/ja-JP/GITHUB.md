# GitHub公開と貢献

[English](../en-US/RELEASING.md) · [简体中文](../zh-CN/GITHUB.md) · **日本語** · [한국어](../ko-KR/GITHUB.md) · [繁體中文](../zh-TW/GITHUB.md) · [Documentation](../README.md)

## 現在の決定

公開ソースリポジトリは[lorvanto/monitor-go](https://github.com/lorvanto/monitor-go)、表示名はMonitor Goです。リリースタグと対象プラットフォームは今後決定します。アプリケーション `0.4.0`、SQLiteスキーマ `4` は開発基準であり、公開済みリリースを意味しません。ソースのpushと、タグや配布ファイルを伴うRelease公開は別に管理します。

プロジェクトのコードと文書は[Apache-2.0](../../LICENSE)で提供します。[NOTICE](../../NOTICE)と[第三者ライセンス一覧](../../THIRD_PARTY_NOTICES.md)も確認してください。依存ライブラリー、フォント、アイコンには個別の条件が適用されます。ライセンス本文は公式英語版を保持します。

## 文書の入口

| 目的 | 文書 |
|---|---|
| インストールと運用 | [利用ガイド](README.md) |
| 開発とPR | [Contributing](../../CONTRIBUTING.md) |
| 問い合わせと障害対応 | [Support](../../SUPPORT.md) |
| 脆弱性の非公開報告 | [Security](../../SECURITY.md) |
| 変更履歴と今後の候補 | [Changelog](../../CHANGELOG.md)・[Roadmap](../../ROADMAP.md) |
| 公開手順 | [Release preparation](../en-US/RELEASING.md) |
| 翻訳の追加・修正 | [翻訳の保守](INTERNATIONALIZATION.md) |

## 開発と検証

Go 1.26.9以降の互換バージョン、Node.js 22.12+ / 24.x、npm、Makeを使います。コマンドはリポジトリ直下のPOSIXシェルで実行します。WindowsではWSLを利用できます。

```sh
make build
./bin/monitor -data tmp/development -listen 127.0.0.1:5093
# 別のターミナル:
MONITOR_GO_API_TARGET=http://127.0.0.1:5093 npm --prefix web run dev
```

`tmp/development/setup-token`をローカルで確認し、`http://127.0.0.1:5188`から初期化します。Goは画面資産を埋め込むため、Goのビルド／テストより先に`web/dist`を生成してください。バックエンド変更後は再ビルドと再起動が必要です。本番データは使用しません。

```sh
make test
npm --prefix web run format:check
# 文書だけを変更した場合:
npm --prefix web run check:docs
```

Go変更では`gofmt`と`go test ./...`も実行します。API変更は独立したサービスへのHTTPで確認し、ポート番号をPRに記載します。画面変更は実ブラウザーのデスクトップ／狭い画面、関連権限、5言語を確認してください。文書チェックはNode.jsだけで実行できます。

SQLiteのみを保存先とし、リソースはインスタンス内で共有します。作成者は監査情報です。書き込みAPIには権限検査を設け、メニューの非表示だけに依存しないでください。ロックファイルを維持し、依存追加時はバージョン、用途、ライセンスを記録します。

PRには問題、最終動作、実行した検証、互換性や移行への影響を記載します。未実施の検査を成功扱いにしません。画面画像にはダミーデータを使い、利用者に影響する変更をUnreleasedへ追記します。大きな変更は先に範囲を相談してください。翻訳時は5言語のキーと置換変数を揃え、API識別子や保存データを変更しません。

提出する貢献にはApache-2.0の貢献条項が適用されます。第三者の表示を保持し、提供権限のある内容だけを提出してください。追加のCLA／DCO手続きは構成されていません。相互に敬意を払い、人物ではなく問題と根拠を議論します。

## 不具合と安全性

通常のIssueにはバージョン／コミット、OS、導入方式、ブラウザー、言語、再現手順、期待結果と実際の結果を記載します。データベース、秘密鍵、Cookie、APIキー、登録トークン、Agent設定は添付しません。

このリポジトリでは非公開脆弱性報告が有効です。[Report a vulnerability](https://github.com/lorvanto/monitor-go/security/advisories/new)、または**Security → Advisories → Report a vulnerability**を使います。ボタンがなければ、詳細を公開せず非公開連絡先だけをIssueで尋ねてください。`SECURITY.md`だけでは機能は有効になりません。専用メール、チャット、対応時間の保証は現在設定されていません。

## 公開前の確認

1. 公開リポジトリは`lorvanto/monitor-go`です。Go moduleと内部importは`github.com/lorvanto/monitor-go`に統一しています。
2. ステージ済みファイルと実際の内容を確認します。`.gitignore`は追跡済みファイルや別パスの秘密情報を除去しません。実データ、バックアップ、秘密情報、個人の画面画像を公開しません。
3. 隣接リポジトリなしのクリーンなチェックアウトで構築・検証します。Issuesと非公開報告は有効化済みです。ブランチ保護とCIは未設定で、必要に応じて構成・確認します。テンプレートはこれらを自動設定しません。
4. 公開リポジトリは作成済みです。今後の同期では変更内容を確認し、remoteが`https://github.com/lorvanto/monitor-go.git`を指していることを確かめて、承認されたブランチをpushします。新しいリポジトリの作成手順は別のリポジトリを作る場合のみ適用します。タグ、配布ファイル、ダウンロードリンクは実際に公開・検証してから追加します。

バイナリー公開では対象機上の実行、Agent再接続、権限、ファイル、通知、停止と復旧を確認します。クロスコンパイル成功だけでは実行保証になりません。全データディレクトリを停止後にバックアップし、テストDBでアップグレードとロールバックを演習してください。

バージョンはフロントエンドのpackage/lockfileと`internal/app/server.go` / `internal/app/settings.go`、変更履歴に同期します。現在`-version`オプションやリンク時の版注入はありません。アーカイブにはバイナリー、起動方法、LICENSE／NOTICE、対象成果物の間接依存も含む必要な第三者表示を同梱し、SHA-256と検証記録を作成します。`make build`はアーカイブ、署名、コンテナーを自動生成しません。

[Releaseテンプレート](../../.github/RELEASE_TEMPLATE.md)で草稿を作り、実際の添付ファイル、チェックサム、互換性、既知の制限を確認してから公開します。文書や過去の検証結果は、新しい候補版の実測を代替しません。

コンテナー変更では`docker compose config --quiet`と`node scripts/test-docker.mjs`を実行します。データ保持、ホスト指標の範囲、ローカルビルドは[Docker導入](DOCKER.md)を参照してください。レジストリー公開は別の公開作業として検証します。

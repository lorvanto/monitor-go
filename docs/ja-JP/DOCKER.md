# Dockerでの導入

[English](../en-US/DOCKER.md) · [简体中文](../zh-CN/DOCKER.md) · **日本語** · [한국어](../ko-KR/DOCKER.md) · [繁體中文](../zh-TW/DOCKER.md) · [Documentation](../README.md)

Docker Engine / Docker DesktopのLinuxコンテナー、BuildKit、Docker Compose v2.20以降を使います。このチェックアウトからビルドし、`monitor-go:local`はローカルタグです。公開済みのレジストリーイメージではありません。Dockerでの構築と実行にはホスト側のGo／Node.jsは不要です。


`make docker-build`はGit由来のバージョンを注入します。Composeを直接使う場合は`MONITOR_VERSION`を指定でき、省略時は`0.4.0-dev`です。直接ビルドする場合は`docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`を使えます。Git履歴はDockerビルドコンテキストに含めません。

## ビルドと初期化

```sh
git clone https://github.com/lorvanto/monitor-go.git
cd monitor-go
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

[ローカルサービス](http://127.0.0.1:5080/)を開き、取得した初期化コードで最初の管理者を作成します。初回表示は英語で、初期化後にコードファイルを削除します。既定の公開先は`127.0.0.1`のみです。5080が使用中の場合はホストポートを変更します。

```sh
MONITOR_PORT=5081 docker compose up -d --build
```

## データと実行範囲

名前付きボリューム`monitor-data`は`/data`全体を保持します。SQLiteと付随ファイル、`master.key`、初期化状態、アップロードを含み、チェックアウトの`data/`とは独立しています。1ボリュームにつき1サービスだけ実行します。`docker compose down`はデータを保持し、`docker compose down -v`は削除します。再作成と更新では同じボリュームを使います。

UID/GIDは`10001:10001`です。Composeではルートファイルシステムが読み取り専用で、`/data`と一時`/tmp`だけを書き込みに使用し、`/readyz`で稼働を確認します。新規ボリュームには準備済みの所有権が適用されます。bind mountを使う場合はUID/GID 10001が書き込める0700の専用ディレクトリを用意し、稼働中のネイティブインスタンスは共有しません。

コンテナー内のホスト採集は既定で無効です。Dockerホストを含む実際のホストにネイティブAgentを設置して登録します。`MONITOR_COLLECT_HOST=true`の指標はコンテナー／VMの環境を反映し、物理ホストと同じとは限りません。サービス監視の`127.0.0.1`もコンテナー自身です。到達可能なDNS/IPを使い、Docker Desktopでは`host.docker.internal`でホストサービスへ接続できます。Docker socket、ホストPID名前空間、privilegedモードは不要です。

## 設定とHTTPS

Composeはshell環境または追跡しない`.env`から`MONITOR_IMAGE`（`monitor-go:local`）、`MONITOR_PORT`（5080）、`MONITOR_BIND_ADDRESS`（127.0.0.1）、`MONITOR_PUBLIC_URL`（空）、`MONITOR_SECURE_COOKIE`（false）、`MONITOR_COLLECT_HOST`（false）、`MONITOR_TRUSTED_PROXY`（空）を読みます。以降のコマンドでも同じ設定を使います。ヘルスチェック用の内部ポートは5080を保持します。

リモートのブラウザー／Agentには、ホストポートの前に信頼されたHTTPSプロキシを置き、末尾スラッシュなしの外部オリジンを設定します。

```sh
MONITOR_PUBLIC_URL=https://monitor.example.com MONITOR_SECURE_COOKIE=true \
  docker compose up -d --build
```

この例はホスト上のプロキシを想定します。他の端末からポートへ接続する場合は`MONITOR_BIND_ADDRESS`を明示します。私設CAはコンテナーの信頼ストアにも導入してください。ホストの証明書設定は自動継承しません。

## Composeを使わない場合

```sh
docker build -t monitor-go:local .
docker volume create monitor-go-data
docker run -d --name monitor-go --restart unless-stopped \
  -p 127.0.0.1:5080:5080 --mount source=monitor-go-data,target=/data \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --stop-timeout 20 monitor-go:local
docker exec monitor-go cat /data/setup-token
```

## 停止後のバックアップと復元

書き込みを停止してボリューム全体を保存します。一時コンテナーも通常と同じ非rootユーザーで停止済みデータを読みます。バックアップは秘密情報として保管してください。

```sh
mkdir -p backups
docker compose stop monitor
docker compose run --rm --no-deps -T --entrypoint tar monitor \
  -C /data -czf - . > backups/monitor-go-data.tar.gz
docker compose start monitor
```

新しいComposeプロジェクトの空ボリュームへ復元します。既存データがあると上書きを拒否し、実行ユーザー（UID/GID 10001）で復元ファイルを作成します。新しいDockerホストでは互換イメージを先にビルドします。元のインスタンスと復元先を同じインストールとして同時運用しないでください。

```sh
docker compose -p monitor-go-restore run --rm --no-deps -T \
  --entrypoint sh monitor -c \
  'test -z "$(ls -A /data)" && tar -xzf - -C /data' \
  < backups/monitor-go-data.tar.gz
MONITOR_PORT=5081 docker compose -p monitor-go-restore up -d --no-build
```

## 更新とロールバック

更新前に停止後の完全バックアップと復元を確認し、レビュー済みソースを更新して再ビルドします。

```sh
git pull --ff-only
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 monitor
```

Composeプロジェクト名とボリュームを保持し、更新時に削除しません。起動時にSQLiteを移行します。ロールバックには旧イメージ／ソースと移行前の完全バックアップが必要です。旧バイナリーが新スキーマを読めるとは限りません。

## 検証とイメージの保守

スモークテストはテスト機にNode.js 22.12+ / 24.xも必要です。専用の一時Composeプロジェクト、ボリューム、自動選択したループバックポートを使い、初期化、権限、ヘルスチェック、アップロードとSHA-256、SQLite／セッション保持、再作成、停止後のバックアップ／復元を検証します。テスト資源を後片付けし、ローカルイメージを残します。

```sh
docker compose config --quiet
node scripts/test-docker.mjs
```

対象を明示するテストでは`MONITOR_PLATFORM=linux/amd64`または`linux/arm64`と、別の`MONITOR_IMAGE`タグを指定します。非ネイティブ実行にはDockerエンジン側のエミュレーター対応が必要です。クロスビルドだけでは実行検証になりません。

Node、Go、Alpineの段階を分離し、実行イメージにビルドツールを含めません。npm／Goのロックファイルと版を指定した基礎イメージを使います。版と用途は[第三者表示](../../THIRD_PARTY_NOTICES.md)を参照してください。証明書とタイムゾーンはAlpine由来で、配備時に基礎イメージとOSパッケージを再確認します。`NODE_IMAGE`、`GO_IMAGE`、`RUNTIME_IMAGE`でレビュー済みの代替を指定できます。BuildKitのクロスビルドに対応しますが、実行保証には対象別の検証が必要です。レジストリーへのpush、タグ作成、自動公開は行いません。

参考: [Dockerボリューム](https://docs.docker.com/engine/storage/volumes/) · [複数プラットフォームのビルド](https://docs.docker.com/build/building/multi-platform/).

## 信頼するプロキシとログイン制限

Compose は `MONITOR_TRUSTED_PROXY` を `-trusted-proxy` に渡します。直接接続では空のままにしてください。コンテナから見える実際のプロキシ CIDR だけを指定し、プライベートネットワーク全体を安易に信頼しないでください。確認済みのプロキシが `172.20.0.2` の場合は `MONITOR_TRUSTED_PROXY=172.20.0.2/32` です。ホストからコンテナへの転送で接続元が変わる場合があるため、先にネットワークを確認してください。

`-trusted-proxy` はカンマ区切りのプロキシ CIDR を受け取り、既定は空（信頼なし）です。不正な CIDR は起動エラーになります。このプロセスから見える実際のプロキシのみを指定し、プロキシで `X-Forwarded-For` を上書きするか安全に追記してください。サーバーは右から左へ解析し、最初の信頼対象外のアドレスを使います。他の接続元のヘッダーは無視します。ログインの失敗だけを集計し、5 分間にクライアント IP ごとに 30 回、正規化したユーザー名ごとに 10 回までです。成功しても回数を消費・リセットしません。初期設定ではすべての試行を集計します（IP ごとに 5 分間で 10 回）。

## オンラインバックアップと復元

コマンド実行中もサービスを継続できます。出力先は存在しないディレクトリとし、親ディレクトリを先に作成します。ソースを読み取り専用で開き、インスタンスロックの取得や移行は行いません（スキーマ 3 から現行版まで対応）。整合性のある SQLite スナップショット、主鍵、スナップショットが参照する全オブジェクト（ごみ箱を含む）を保存します。以降の新規アップロードは含みません。欠落を再試行し、欠落・破損・変更が残れば非ゼロ終了と BACKUP_INCOMPLETE を残します。このディレクトリは復元できません。完了マニフェストには UTC 時刻、ビルド/スキーマ、件数と SHA-256 を記録します。初期化トークン、ロック、一時ファイル、画像キャッシュは除外します。

コンテナ内のデータボリュームに新規スナップショットを作成し、直ちに独立した保存先へコピーします。毎回一意のディレクトリ名を使います。作成したアーカイブは下記の空ボリュームへの復元手順で利用できます。

```sh
mkdir -p backups
docker compose exec monitor mkdir -p /data/backups
docker compose exec monitor monitor backup -data /data -out /data/backups/snapshot-001
docker compose exec -T monitor tar -C /data/backups/snapshot-001 -czf - . > backups/monitor-go-online.tar.gz
```

復元時はサービスを停止し、データディレクトリ全体を完全なバックアップに置き換え、そのスキーマ以上に対応するバイナリで起動します。検証完了まで旧ディレクトリを保持してください。復元はオフラインのみで、稼働中のデータに混在させません。認証情報を含むため非公開で保管し、独立したストレージにコピーします。コールドバックアップも引き続き利用できます。

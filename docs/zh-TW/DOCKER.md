# Docker 部署

[English](../en-US/DOCKER.md) · [简体中文](../zh-CN/DOCKER.md) · [日本語](../ja-JP/DOCKER.md) · [한국어](../ko-KR/DOCKER.md) · **繁體中文** · [Documentation](../README.md)

使用 Docker Engine / Docker Desktop 的 Linux 容器模式、BuildKit 及 Docker Compose v2.20 或更新版本。映像檔從目前原始碼建置，`monitor-go:local` 是本機標籤，並非已發佈的登錄檔映像。透過 Docker 建置與執行時，宿主機無須安裝 Go 或 Node.js。


`make docker-build` 會注入 Git 衍生版本。直接使用 Compose 時可傳入 `MONITOR_VERSION`，未指定則顯示 `0.4.0-dev`。直接建置可用 `docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`。Git 歷史不進入 Docker 建置內容。

## 建置與初始化

```sh
git clone https://github.com/lorvanto/monitor-go.git
cd monitor-go
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

開啟 [本機服務](http://127.0.0.1:5080/)，使用讀取到的初始化碼建立首個管理員。首次介面預設英文，初始化後刪除碼檔案。Compose 預設只綁定 `127.0.0.1`。若 5080 已被佔用，可改用其他宿主機連接埠：

```sh
MONITOR_PORT=5081 docker compose up -d --build
```

## 資料與執行範圍

具名磁碟區 `monitor-data` 持久保存整個 `/data`，包含 SQLite 及伴隨檔案、`master.key`、初始化狀態和上傳檔案，與原始碼目錄的 `data/` 獨立。每個磁碟區只執行一個服務程序。`docker compose down` 保留磁碟區；`docker compose down -v` 會刪除磁碟區。重建容器或升級映像會沿用原有資料。

容器使用 UID/GID `10001:10001`，Compose 設定唯讀根檔案系統，可寫區域為 `/data` 與暫存 `/tmp`，以 `/readyz` 檢查就緒狀態。新具名磁碟區繼承已準備的目錄權限。自訂 bind mount 時，先建立 UID/GID 10001 可寫、權限 0700 的專用目錄，不可掛載正在執行的原生實例目錄。

預設關閉容器內的本機指標採集。請在真實主機（包括 Docker 宿主機）執行原生 Agent，並從介面接入。可選 `MONITOR_COLLECT_HOST=true` 只會採集容器或虛擬機環境，不保證反映實體主機。服務探測在容器內執行，`127.0.0.1` 指向容器本身；請使用可達的服務 DNS/IP。Docker Desktop 可透過 `host.docker.internal` 存取宿主機服務。無須 Docker socket、宿主機 PID 命名空間或 privileged 模式。

## 設定與 HTTPS

Compose 讀取 shell 環境變數或本機未追蹤的 `.env`：`MONITOR_IMAGE`（預設 `monitor-go:local`）、`MONITOR_PORT`（5080）、`MONITOR_BIND_ADDRESS`（127.0.0.1）、`MONITOR_PUBLIC_URL`（空）、`MONITOR_SECURE_COOKIE`（false）、`MONITOR_COLLECT_HOST`（false）及 `MONITOR_TRUSTED_PROXY`（空）。後續 Compose 命令須保持相同設定，容器內部連接埠維持 5080 供健康檢查使用。

遠端瀏覽器與 Agent 存取應在宿主機連接埠前部署可信的 HTTPS 反向代理，並設定不帶結尾斜線的實際外部來源：

```sh
MONITOR_PUBLIC_URL=https://monitor.example.com MONITOR_SECURE_COOKIE=true \
  docker compose up -d --build
```

範例假設代理執行於宿主機。若其他機器需要存取映射連接埠，明確設定 `MONITOR_BIND_ADDRESS`。私有 CA 還需安裝或掛載至容器信任庫，不會自動繼承宿主機憑證設定。

## 不使用 Compose

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

## 停機備份與還原

先停止寫入，再封存整個磁碟區。暫時的輔助容器使用相同的非 root 執行使用者讀取停機資料。備份包含憑據與業務資料，請私密保存。

```sh
mkdir -p backups
docker compose stop monitor
docker compose run --rm --no-deps -T --entrypoint tar monitor \
  -C /data -czf - . > backups/monitor-go-data.tar.gz
docker compose start monitor
```

還原至全新的 Compose 專案與空磁碟區。命令拒絕覆寫既有資料，並以執行使用者（UID/GID 10001）建立還原檔案。在新 Docker 主機上先建置相容的本機映像。不要將原實例與還原實例同時當作同一套安裝執行。

```sh
docker compose -p monitor-go-restore run --rm --no-deps -T \
  --entrypoint sh monitor -c \
  'test -z "$(ls -A /data)" && tar -xzf - -C /data' \
  < backups/monitor-go-data.tar.gz
MONITOR_PORT=5081 docker compose -p monitor-go-restore up -d --no-build
```

## 升級與回復舊版

升級前先完成並驗證停機備份，再更新審查過的原始碼並重新建置：

```sh
git pull --ff-only
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 monitor
```

保持 Compose 專案名稱與磁碟區一致，升級時不要刪除磁碟區。SQLite 在啟動時執行版本移轉。回復舊版需要舊映像或原始碼及升級前的完整備份，舊程式可能無法讀取新版資料庫。

## 驗證與映像維護

冒煙測試另需測試機器安裝 Node.js 22.12+ / 24.x。腳本使用暫時的 Compose 專案與磁碟區，自動選擇回環連接埠，驗證初始化、權限、健康檢查、上傳位元組與 SHA-256、SQLite/工作階段持久化、容器重建及停機備份還原。完成後清理自身測試專案，保留本機原始碼映像。

```sh
docker compose config --quiet
node scripts/test-docker.mjs
```

指定測試架構時設定 `MONITOR_PLATFORM=linux/amd64` 或 `linux/arm64`，並使用獨立的 `MONITOR_IMAGE` 標籤。執行非本機架構需要 Docker 引擎支援對應模擬器；交叉建置本身不等於執行驗證。

建置分為 Node、Go 和 Alpine 階段，執行映像不包含編譯工具。npm/Go 鎖定檔與版本化基礎映像鎖定建置輸入，版本和用途見 [第三方聲明](../../THIRD_PARTY_NOTICES.md)。執行時憑證與時區資料來自 Alpine，部署時應重新建置並核查基礎映像與系統套件。`NODE_IMAGE`、`GO_IMAGE`、`RUNTIME_IMAGE` 建置參數用於經審查的替換。支援 BuildKit 交叉建置，承諾的執行平台仍需分別實測。這些命令不推送登錄檔、不建立版本標籤，也不自動發佈。

參考: [Docker 磁碟區](https://docs.docker.com/engine/storage/volumes/) · [多平台建置](https://docs.docker.com/build/building/multi-platform/).

## 可信代理與登入限流

Compose 將 `MONITOR_TRUSTED_PROXY` 傳給 `-trusted-proxy`；直接連線時保持空白。依容器實際看到的代理位址設定 CIDR，不要任意信任整個私有網路。例如，已確認代理位址為 `172.20.0.2` 時使用 `MONITOR_TRUSTED_PROXY=172.20.0.2/32`。宿主機至容器的轉送可能改變來源位址，應先確認網路再啟用信任。

`-trusted-proxy` 接受逗號分隔的代理 CIDR，預設留空（不信任代理），無效值會阻止啟動。只填寫本程序實際看到的代理位址；代理必須覆寫或安全追加 `X-Forwarded-For`。伺服器從右向左尋找首個不可信位址，忽略其他來源的轉送標頭。登入只統計失敗：每個用戶端 IP 每 5 分鐘 30 次、每個正規化使用者名稱每 5 分鐘 10 次；成功登入不消耗也不重設計數。初始化對所有嘗試計數（每個 IP 每 5 分鐘 10 次）。

# Docker 部署

[English](../en-US/DOCKER.md) · [简体中文](../zh-CN/DOCKER.md) · [日本語](../ja-JP/DOCKER.md) · [한국어](../ko-KR/DOCKER.md) · **繁體中文** · [Documentation](../README.md)

使用 Docker Engine / Docker Desktop 的 Linux 容器模式、BuildKit 及 Docker Compose v2.20 或更新版本。映像檔從目前原始碼建置，`monitor-go:local` 是本機標籤，並非已發佈的登入檔映像。透過 Docker 建置與執行時，宿主機無須安裝 Go 或 Node.js。

映像包含以同一原始碼建置的 Linux amd64/arm64 Agent 下載包。在主機詳情選擇「Linux 一鍵安裝」，即可於目標伺服器安裝原生 systemd Agent，目標機無須安裝 Go。下載包由此實例提供，不表示已發佈發行版本。


`make docker-build` 會注入 Git 衍生版本。直接使用 Compose 時可傳入 `MONITOR_VERSION`，未指定則顯示 `0.4.0-dev`。直接建置可用 `docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`。Git 歷史不進入 Docker 建置內容。

## 建置與初始化

```sh
git clone https://github.com/lorvanto/monitor-go.git
cd monitor-go
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

首次安裝顯示 Console 目的地、欄位及持續回報規則，明確確認、線上登記成功後才建立本機管理員。之後回報持續啟用，沒有關閉開關。既有啟用的執行個體保留身分及歷史同意；舊版關閉的執行個體等待管理員確認，不中斷本機監控。Console 故障不阻斷登入、監控或告警，帳號綁定仍自願。舊的 activation=false 設定不再支援。 [首次啟用與使用情況回報](USAGE_REPORTING.md)

開啟 [本機服務](http://127.0.0.1:5080/)，使用讀取到的初始化碼建立首個管理員。首次介面預設英文，初始化後刪除碼檔案。Compose 預設只綁定 `127.0.0.1`。若 5080 已被佔用，可改用其他宿主機連接埠：

```sh
MONITOR_PORT=5081 docker compose up -d --build
```

## 資料與執行範圍

具名磁碟區 `monitor-data` 持久保存整個 `/data`，包含 SQLite 及伴隨檔案、`master.key`、初始化狀態和上傳檔案，與原始碼目錄的 `data/` 獨立。每個磁碟區只執行一個服務程序。`docker compose down` 保留磁碟區；`docker compose down -v` 會刪除磁碟區。重建容器或升級映像會沿用原有資料。

容器使用 UID/GID `10001:10001`，Compose 設定唯讀根檔案系統，可寫區域為 `/data` 與暫存 `/tmp`，以 `/readyz` 檢查就緒狀態。新具名磁碟區繼承已準備的目錄權限。自訂 bind mount 時，先建立 UID/GID 10001 可寫、權限 0700 的專用目錄，不可掛載正在執行的原生實例目錄。

預設關閉容器內的本機指標採集。請在真實主機（包括 Docker 宿主機）執行原生 Agent，並從介面接入。可選 `MONITOR_COLLECT_HOST=true` 只會採集容器或虛擬機環境，不保證反映實體主機。服務探測在容器內執行，`127.0.0.1` 指向容器本身；請使用可達的服務 DNS/IP。Docker Desktop 可透過 `host.docker.internal` 存取宿主機服務。無須 Docker socket、宿主機 PID 命名空間或 privileged 模式。

## 設定與 HTTPS

Compose 讀取 shell 環境變數或本機未追蹤的 `.env`：`MONITOR_IMAGE`（預設 `monitor-go:local`）、`MONITOR_PORT`（5080）、`MONITOR_BIND_ADDRESS`（127.0.0.1）、`MONITOR_PUBLIC_URL`（空）、`MONITOR_SECURE_COOKIE`（false）、`MONITOR_COLLECT_HOST`（false）、`MONITOR_TRUSTED_PROXY`（空）及 `MONITOR_REQUIRE_ACTIVATION`（只兼容 `true`）、`MONITOR_CONSOLE_URL`（未設定時使用預先指定的 Console 來源，實際位址以安裝或確認頁面顯示為準；空值或純空白值是無效設定）。後續 Compose 命令須保持相同設定，容器內部連接埠維持 5080 供健康檢查使用。

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

升級前先完成並驗證停機備份，再更新審查過的原始碼並重新增置：

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

建置分為 Node、Go 和 Alpine 階段，執行映像不包含編譯工具。npm/Go 鎖定檔與版本化基礎映像鎖定建置輸入，版本和用途見 [第三方聲明](../../THIRD_PARTY_NOTICES.md)。執行時憑證與時區資料來自 Alpine，部署時應重新增置並核查基礎映像與系統套件。`NODE_IMAGE`、`GO_IMAGE`、`RUNTIME_IMAGE` 建置參數用於經審查的替換。支援 BuildKit 交叉建置，承諾的執行平台仍需分別實測。這些命令不推送登入檔、不建立版本標籤，也不自動發佈。

參考: [Docker 磁碟區](https://docs.docker.com/engine/storage/volumes/) · [多平台建置](https://docs.docker.com/build/building/multi-platform/).

## 可信代理與登入限流

Compose 將 `MONITOR_TRUSTED_PROXY` 傳給 `-trusted-proxy`；直接連線時保持空白。依容器實際看到的代理位址設定 CIDR，不要任意信任整個私有網路。例如，已確認代理位址為 `172.20.0.2` 時使用 `MONITOR_TRUSTED_PROXY=172.20.0.2/32`。宿主機至容器的轉送可能改變來源位址，應先確認網路再啟用信任。

`-trusted-proxy` 接受逗號分隔的代理 CIDR，預設留空（不信任代理），無效值會阻止啟動。只填寫本程序實際看到的代理位址；代理必須覆寫或安全追加 `X-Forwarded-For`。伺服器從右向左尋找首個不可信位址，忽略其他來源的轉送標頭。登入只統計失敗：每個用戶端 IP 每 5 分鐘 30 次、每個正規化使用者名稱每 5 分鐘 10 次；成功登入不消耗也不重設計數。初始化對所有嘗試計數（每個 IP 每 5 分鐘 10 次）。

## 線上備份與還原

執行命令時服務可繼續運行。輸出目錄必須不存在，父目錄必須已存在。命令唯讀開啟來源資料庫，不取得執行個體鎖、不執行移轉（支援結構版本 3 至目前版本），產生一致的 SQLite 快照並複製主金鑰與快照引用的全部物件，包括回收桶檔案。快照之後的新上傳不納入備份。遺失物件會重試；仍遺失、損壞或變更時以非零狀態結束並保留 BACKUP_INCOMPLETE，不能還原該目錄。完成清單記錄 UTC 時間、建置/結構版本、物件數量及 SHA-256。初始化碼、鎖、暫存檔與圖片快取不複製。

容器命令先在資料卷中產生新快照，隨後應立即複製到獨立儲存。每次使用唯一目錄名稱。範例封存檔可按下方新資料卷還原流程操作。

```sh
mkdir -p backups
docker compose exec monitor mkdir -p /data/backups
docker compose exec monitor monitor backup -data /data -out /data/backups/snapshot-001
docker compose exec -T monitor tar -C /data/backups/snapshot-001 -czf - . > backups/monitor-go-online.tar.gz
```

還原前先停服，以完整備份取代整個資料目錄，再使用支援該結構版本或更高版本的程式啟動。驗證成功前保留舊目錄。僅支援離線還原，不要將檔案合併至運行中的執行個體。備份包含憑據，應私密保存並複製到獨立儲存；原有冷備份仍可使用。

## 首次啟用與使用情況回報

首次安裝顯示 Console 目的地、欄位及持續回報規則，明確確認、線上登記成功後才建立本機管理員。之後回報持續啟用，沒有關閉開關。既有啟用的執行個體保留身分及歷史同意；舊版關閉的執行個體等待管理員確認，不中斷本機監控。Console 故障不阻斷登入、監控或告警，帳號綁定仍自願。舊的 activation=false 設定不再支援。

`MONITOR_CONSOLE_URL` 未設定時，原生程式及 Compose 使用預先指定的 Console 來源，實際位址以安裝或確認頁面顯示為準；可設為其他 HTTPS 來源以更換目的地，原生 `-console-url` 參數優先。空值或只有空白的目的地（包括環境變數提供的值）屬於無效設定，標準 CLI 會拒絕啟動，不能用於關閉回報或略過啟用。版本查詢與備份命令不依賴啟用設定。

標準 CLI 與 Compose 必須啟用並回報。舊的 `-require-activation=false` 及 `MONITOR_REQUIRE_ACTIVATION=false` 設定會明確報錯，不再支援；`true` 保留相容。沒有受支援的離線安裝或關閉回報參數。應設定可達的 HTTPS Console 網址，空值或只有空白的目的地屬於無效啟動設定，不能用於關閉回報或略過啟用要求。Console URL 參數優先於對應環境變數；既有 activation=false 設定不能透過另加 true 參數變為有效。

原生程式預設使用 `native`；Docker 映像和 Compose 自動設定為 `docker`。

僅支援 HTTPS；數字迴環位址的 HTTP 僅供本機整合測試。容器中的迴環位址指向容器本身，分開部署的容器應連線真實 HTTPS 網址。不要將憑據、資料目錄或備份提交 Git；反向代理記錄應避免儲存 OAuth 查詢參數、完整來源 IP 及憑據。

[首次啟用與使用情況回報](USAGE_REPORTING.md)

# 首次啟用與使用情況回報

[English](../en-US/USAGE_REPORTING.md) · [简体中文](../zh-CN/USAGE_REPORTING.md) · [日本語](../ja-JP/USAGE_REPORTING.md) · [한국어](../ko-KR/USAGE_REPORTING.md) · [繁體中文](../zh-TW/USAGE_REPORTING.md)

Monitor Go 與 Monitor Console 是獨立儲存庫、服務及資料庫，原始碼與資料目錄分別放在同層目錄。安裝 Monitor Go 只需此公開儲存庫，不需要私有後台原始碼。標準首次安裝會連線至設定的 Console 完成啟用；後續登入、監控和告警獨立執行。

標準新安裝須先線上啟用，再建立首位本機管理員。安裝頁明確顯示目的地（預設為 `https://feiniu.i7f.cn:5090`）、統計用途及具體欄位，須手動勾選確認。執行個體登記成功後才建立管理員並啟用回報；不會傳送本機使用者名稱、顯示名稱或密碼。登記失敗時不建立管理員，修復網路後可用同一執行個體身分重試。啟用不需 Console 帳號、GitHub 登入，也不會自動綁定開發者帳號。

已初始化執行個體升級時不強制啟用，也不默默開啟回報；對同一接收方保留既有選擇。不含此功能的舊執行個體仍保持關閉，具有 `system:settings` 權限的管理員可之後查看欄位並自行開啟。更換接收方須重新同意。啟用後仍可關閉回報，Console 離線不會阻斷登入、監控或告警。

回報僅包含隨機執行個體編號、版本、系統類型、處理器架構、部署方式，以及已登入使用者實際操作介面的 UTC 日期。不包含本機帳號清單、監控目標、主機名稱/IP、檔案、告警內文、密碼、監控金鑰或應用程式金鑰。執行個體憑據僅用於登記及請求驗證，不顯示於介面。後台帳號獨立於 Monitor Go：本機 Console 帳號儲存使用者名稱及顯示名稱；選擇 GitHub 登入時另外儲存經驗證的 GitHub 編號、使用者名稱及顯示名稱。

執行個體登記及活躍統計不需綁定帳號。Console 支援本機管理員使用者名稱與密碼登入，GitHub 登入為選用功能；安裝 Monitor Go 不需設定 OAuth。

如需關聯帳號，先使用本機 Console 帳號或已啟用的 GitHub 登入 Monitor Console，產生 10 分鐘有效的一次性綁定碼，再貼到 Monitor Go 的回報設定卡片。需先開啟回報。解除綁定僅移除帳號關聯，不會關閉回報；也可在後台解除自己名下執行個體的關聯。

「上次同步的 Console 帳號」是本機儲存的快照。在 Monitor Console 端解除綁定後，Monitor Go 要等到下一次成功回報才會更新此顯示。關閉回報期間不會聯絡後台，因此可能繼續顯示舊帳號。「重新整理回報狀態」只重新讀取本機設定，不會請求後台或啟動遠端同步。

開啟後獨立工作先回報一次，其後約每 6 小時回報並加入隨機間隔。單次請求 5 秒逾時，不跟隨重新導向，失敗採有限退避重試；最多保留 7 個 UTC 日期的待傳送活躍標記。自動更新和單純載入頁面不計入人工活躍。每日按執行個體去重，不代表獨立使用者人數；這些是自行回報的使用情況估算，無法證明身分或完整安裝量。

若遠端登記成功，但本機建立管理員的交易失敗或安裝遭取消，後台可能保留未綁定的隨機執行個體記錄；重試沿用同一身分。因此登記數不等於已完成安裝數或實際使用者人數。

可隨時關閉：取消進行中的請求、清空待傳送活躍記錄，監控、告警和登入保持獨立。關閉不刪除後台已收到的歷史記錄。後台每日記錄保留 90 天，365 天未回報的執行個體會被清理；之後重新回報可再次登記，但需要重新綁定帳號。

僅支援 HTTPS；數字迴環位址的 HTTP 僅供本機整合測試。容器中的迴環位址指向容器本身，分開部署的容器應連線真實 HTTPS 網址。不要將憑據、資料目錄或備份提交 Git；反向代理記錄應避免儲存 OAuth 查詢參數、完整來源 IP 及憑據。

結構版本 10 在版本 9 的回報設定上新增告知版本、同意時間及啟用時間。升級前完整備份資料目錄，還原需使用原備份。隨機執行個體身分隨資料目錄持久保存，複製目錄可能合併統計；關閉回報、離線或修改原始碼的安裝無法完整統計。

`MONITOR_CONSOLE_URL` 未設定時，原生程式和 Compose 使用 `https://feiniu.i7f.cn:5090`。可將其設為其他 HTTPS 來源以覆寫目的地，或明確設定 `MONITOR_CONSOLE_URL=` 以取消目的地設定。原生程式的 `-console-url` 參數優先於環境變數；`-console-url=` 同樣會取消目的地設定。

標準 CLI 與 Compose 的 `MONITOR_REQUIRE_ACTIVATION` / `-require-activation` 預設值為 `true`。自建發行版或離線環境可明確設定 `MONITOR_REQUIRE_ACTIVATION=false` 或傳入 `-require-activation=false`；此時在本機完成安裝，回報保持關閉，直到另行開啟。僅清空 Console URL 無法略過必要啟用：須正確設定目的地或明確關閉啟用要求。命令列參數優先於環境變數。

```sh
./bin/monitor -data data -listen 127.0.0.1:5080
```

明確選擇離線安裝的範例：

```sh
./bin/monitor -data data -listen 127.0.0.1:5080 -require-activation=false -console-url=
```

`MONITOR_CONSOLE_URL` / `-console-url`; `MONITOR_REQUIRE_ACTIVATION` / `-require-activation`; `MONITOR_DEPLOYMENT=native|docker` / `-deployment`.

原生程式預設使用 `native`；Docker 映像和 Compose 自動設定為 `docker`。

[README](README.md) · [Docker](DOCKER.md)

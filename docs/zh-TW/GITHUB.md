# GitHub 發佈與協作指南

[English](../en-US/RELEASING.md) · [简体中文](../zh-CN/GITHUB.md) · [日本語](../ja-JP/GITHUB.md) · [한국어](../ko-KR/GITHUB.md) · **繁體中文** · [Documentation](../README.md)

## 已確定與待完成事項

公開原始碼倉庫為 [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go)。當前顯示名稱為 Monitor Go，應用版本為 `0.4.0-dev`，SQLite 結構版本為 `8`。這兩個版本號表示開發基線，不代表已發佈 GitHub Release。項目代碼與文檔採用 [Apache-2.0](../../LICENSE)，第三方依賴保留原許可證。

目前 Go module 與內部匯入已統一為 `github.com/lorvanto/monitor-go`。原始碼推送與版本發佈分開管理；目前不包含 GitHub Release 標籤或下載資產。

## 文檔分工

| 檔案 | 用途 |
|---|---|
| [README](../../README.md) | 面向首次存取者的英文項目入口 |
| [多語言文檔](../README.md) | 五語當前使用指南與維護入口 |
| [貢獻指南](../../CONTRIBUTING.md) | 開發環境、改動邊界、測試、PR 與貢獻許可 |
| [安全政策](../../SECURITY.md) | 私密漏洞反饋方式和部署邊界 |
| [支援說明](../../SUPPORT.md) | 問題反饋資訊與常見故障排查 |
| [變更記錄](../../CHANGELOG.md) | 當前 Unreleased 變更；實際發佈後再新增日期與版本條目 |
| [路線圖](../../ROADMAP.md) | 候選後續工作，不承諾交付日期 |
| [第三方聲明](../../THIRD_PARTY_NOTICES.md) | 鎖定版本、用途、許可證和原始許可文本 |
| [發佈流程](../en-US/RELEASING.md) | 建倉、版本、驗證、打包和發佈檢查 |
| [PR 模板](../../.github/pull_request_template.md) | 問題、最終行為、實際驗證與兼容性 |
| [Release 模板](../../.github/RELEASE_TEMPLATE.md) | 發佈說明、下載、校驗和、升級與限制 |

Issue 模板在 `.github/ISSUE_TEMPLATE/`，分別用於缺陷、功能建議和文檔翻譯。模板不預設尚未建立的標籤或團隊，也不會自動建立 CI、分支保護或 GitHub Release。

## 參與開發

需要 Go 1.26.9 或更新的兼容版本、Node.js 22.12+ / 24.x、npm 與 Make。示例使用 POSIX shell；Windows 開發者可使用 WSL。

```sh
make build
./bin/monitor -data tmp/development -listen 127.0.0.1:5093
# 在第二个终端运行：
MONITOR_GO_API_TARGET=http://127.0.0.1:5093 npm --prefix web run dev
```

從本機 `tmp/development/setup-token` 讀取初始化碼，瀏覽器存取 `http://127.0.0.1:5188`。Go 會嵌入前端資源，運行 Go 構建或測試前應先生成 `web/dist`。後端修改後需要重新構建並重啟。開發和驗收使用隔離 SQLite 檔案，不使用正式資料。

SQLite 是唯一業務存儲；資源由本套安裝共享，建立人字段只作審計。菜單隱藏不能代替伺服器端授權。新寫介面需檢查權限，資料庫變更需驗證真實檔案上的遷移、約束和事務。依賴鎖檔案必須保留，新增依賴記錄用途、版本與許可證。

```sh
make test
npm --prefix web run format:check
```

Go 代碼使用 `gofmt`，Go 實現另運行 `go test ./...`。API 變更啟動隔離服務做真實 HTTP 檢查，PR 中寫明連接埠；界面變更驗證桌面、窄屏和相關權限。純文檔改動運行 `npm --prefix web run check:docs` 即可，該檢查只需 Node.js。檢查未執行時明確說明，不能寫成通過。

提交 PR 時說明問題、最終行為、實際驗證、兼容性和遷移影響。新增文案同步五語目錄，變量、機器字段和使用者內容不能隨顯示語言改變。可見界面變更附脫敏截圖，使用者可感知變更寫入 Unreleased。重大行為調整先討論範圍。貢獻按 Apache-2.0 的貢獻條款處理，保留第三方署名；當前沒有額外 CLA 或 DCO 流程。交流應尊重他人，圍繞問題和證據討論。

## 問題與安全反饋

普通問題使用倉庫 Issue，附版本或提交、系統架構、安裝方式、瀏覽器與界面語言、復現步驟、預期與實際結果。不要提交資料庫、完整日誌、Cookie、API 金鑰、主金鑰、接入碼或 Agent 配置。

倉庫已開啟私密漏洞報告，可透過 [Report a vulnerability](https://github.com/lorvanto/monitor-go/security/advisories/new) 或 **Security → Advisories → Report a vulnerability** 提交安全問題。若入口不存在，只公開詢問私密聯繫方式，不公開漏洞細節或利用步驟。`SECURITY.md` 不會自動開啟該功能，當前也未配置獨立安全郵箱、聊天群或回應時限。具體以 [安全政策](../../SECURITY.md) 為準。

## 首次公開源碼前

- [x] 五語當前指南、貢獻、安全、支援、路線圖和變更記錄已整理。
- [x] Apache-2.0、NOTICE、直接依賴許可證清單與許可文本已加入。
- [x] 缺陷／功能／翻譯 Issue 模板、PR 與 Release 說明模板已加入。
- [x] 確定公開倉庫 `lorvanto/monitor-go`，簡介與 topics 描述目前監控範圍。
- [x] 審查實際暫存檔案與內容；`.gitignore` 不能清理已經跟蹤或存到其他路徑的資料。
- [x] 從暫存原始碼的獨立匯出目錄完成構建和測試，確認無舊目錄、符號連結或本機執行資料依賴。
- [x] module 路徑與內部匯入已同步至正式倉庫地址。
- [x] Issues 和私密漏洞報告已開啟並驗證。
- [ ] 按實際需要配置分支保護和 CI；目前尚未配置。

```sh
git status --short --untracked-files=all
git ls-files
git diff --cached --stat
git diff --cached --check
npm --prefix web run check:docs
```

還應在本機審查實際暫存內容，檢查金鑰、私人地址、機器路徑和真實截圖。示例使用假資料與 example 域名。不要上傳 `data/`、`tmp/`、`backups/`、上傳檔案或憑據。上述命令不能代替完整的敏感資訊檢查。

公開原始碼倉庫已經建立。後續同步前審查待推送內容，確認 remote 指向 `https://github.com/lorvanto/monitor-go.git`，再推送獲准的分支。僅在實際建立其他倉庫時選用建倉步驟；已有本地內容時，避免自動產生另一份 README 或許可證造成無關歷史。版本標籤、發佈資產和下載連結仍需在實際發佈並驗證後加入。

## 發佈下載包前

先確定要承諾的操作系統與架構，完成 `make test`、格式檢查和 `make build`，記錄工具鏈與提交。對候選構建重新執行依賴與可達漏洞檢查，不能沿用歷史“零漏洞”結論。交叉編譯成功不代表目標系統運行通過；目標機需要驗證安裝、初始化、登入、權限、探測、Agent 重連、檔案、通知和停止流程。

升級與回退使用測試資料庫演練。停機完整備份資料庫及伴隨檔案、`master.key` 與原上傳檔案；舊二進制不能直接讀取 schema 8，回退依賴整套升級前備份。確認伺服器端與 Agent 的兼容組合，明確未驗證平台。

選定發佈版本後，同步 `web/package.json`、鎖檔案根包版本、`internal/app/server.go` / `internal/app/settings.go` 返回的版本、變更記錄和使用指南。當前沒有 `-version` 參數或鏈接器版本注入機制，不使用不存在的參數。資料庫版本單獨管理。

發佈封存包含對應平台二進制、啟動說明、項目 LICENSE／NOTICE 與該產物的完整第三方許可資料。當前第三方清單覆蓋直接依賴，發佈二進制前還需按實際鏈接和打包內容核對間接依賴。生成真實封存的 SHA-256，記錄源碼提交、工具鏈和已測平台。`make build` 不自動生成封存、校驗和、簽名或容器鏡像。

先建立草稿 Release，按模板填寫變更、兼容性、下載資產、校驗和、驗證結果與限制，核對後再公開。已提供 [Docker 原始碼建置與 Compose](DOCKER.md)，尚未發佈預建登入檔映像、簽名服務或自動發佈流程；發佈映像前另行核實標籤、許可證與目標平台。

## 文檔維護

修改命令、支援範圍或協作規則時，同步相關五語指南。法律文本保留官方英文原文，其他語言說明不改寫許可條款。歷史設計與驗證資料僅記錄當時範圍；新發佈必須以當前候選代碼和實際驗證為準。

容器變更須通過 `docker compose config --quiet` 與 `node scripts/test-docker.mjs`。持久化、主機指標範圍和本機建置方式見 [Docker 部署](DOCKER.md)。推送登入檔仍須另行執行並核實發佈任務。

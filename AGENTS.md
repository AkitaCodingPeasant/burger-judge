# REGS 專案導覽

這份文件是代理程式的快速索引。處理任務時先從下方對應入口讀起，不要先遞迴掃描整個工作區；只有索引不足時，才用 `rg` 限定目錄搜尋。

## 快速理解專案

REGS 是以 Go 開發的線上評測後端。API 接收請求並透過行程內的非同步 Queue 與 Semaphore 執行 Docker、CMake、CTest 判題。PostgreSQL 保存使用者、題目與提交狀態；檔案系統保存上傳檔和判題日誌。

建議閱讀順序：

1. `AGENTS.md`：檔案與任務索引。
2. `README.md`：啟動、操作與驗證方式。
3. `project.md`：作業原始需求與評分規格（本機參考，不提交 Git）。
4. `go.mod`：Go module 與版本。

## 目錄與檔案職責

| 路徑 | 職責 |
|---|---|
| `cmd/api/main.go` | API 執行檔入口；載入設定並啟動 API。 |
| `internal/app/api.go` | API dependency wiring、HTTP server lifecycle、graceful shutdown。 |
| `internal/config/config.go` | 環境變數、路徑、worker 數量與 timeout。 |
| `internal/domain/auth.go` | 角色與權限規則。 |
| `internal/domain/submission.go` | Submission entity、狀態與狀態判斷。 |
| `internal/usecase/ports.go` | Repository、檔案儲存、JudgeRunner 的介面邊界。 |
| `internal/adapter/httpapi/` | 完整 Router、handler、JWT/RBAC middleware 與 HTTP DTO。 |
| `internal/adapter/authjwt/` | ES256 JWT 金鑰產生、簽署與驗證。 |
| `internal/adapter/postgres/` | PostgreSQL repository 與 migration runner。 |
| `internal/adapter/filestorage/` | ZIP、上傳檔與分階段 log 的儲存實作區。 |
| `internal/adapter/dockerjudge/` | Docker、CMake、CTest 判題實作區。 |
| `migrations/` | PostgreSQL schema；`*.up.sql` 建立，`*.down.sql` 回滾。 |
| `api/openapi.yaml` | OpenAPI 3.0 完整 API 規格。 |
| `compose.yaml` | 本機 PostgreSQL 容器設定。 |
| `README.md` | 環境需求、啟動方式與快速 API 操作。 |
| `docs/erd.md` | PostgreSQL ERD 與資料保存說明。 |
| `docs/demo-guide.md` | 完整 PowerShell Demo 流程與評分項目對照。 |
| `data/` | 本機題目、上傳與建置資料；內容不進 Git。 |
| `docs/basic20-testing.md` | 基本 20 分的自動化與手動驗收步驟。 |
| `docs/advanced10-testing.md` | 進階 10 分 Job Queue、rerun 與 semaphore 驗收步驟。 |
| `test-framework/README.md` | CMake/CTest 題目框架說明。 |
| `test-framework/113final*/` | 各題的 `template/`、`solution/`、`spec/` 與 `cmake/`。 |
| `.env.example` | 可提交的環境變數範本。 |

## 常見任務入口

| 任務 | 優先查看 | 通常連帶修改 |
|---|---|---|
| 新增或修改 API | `internal/adapter/httpapi/` | `internal/app/api.go`、`api/openapi.yaml`、HTTP tests |
| 認證與 RBAC | `internal/domain/auth.go` | `internal/adapter/httpapi/`、`internal/usecase/` |
| Submission 狀態或流程 | `internal/domain/submission.go` | `internal/usecase/`、`internal/adapter/postgres/`、migration |
| 資料庫 schema | `migrations/` | `internal/adapter/postgres/`、domain model |
| Queue 與併發控制 | `internal/usecase/submission_service.go` | `internal/app/api.go`、submission tests |
| ZIP/日誌/路徑處理 | `internal/adapter/filestorage/` | `internal/usecase/ports.go`、`internal/config/config.go` |
| Docker 判題管線 | `internal/adapter/dockerjudge/` | `internal/app/api.go`、`test-framework/` |
| 修改題目或測資 | `test-framework/<題號>/` | 該題 `CMakeLists.txt`、`cmake/AddJudge.cmake`、`spec/` |
| 設定環境變數 | `internal/config/config.go` | `.env.example`、部署設定 |
| 部署 | `deploy/` | `.env.example`、`cmd/`、migration 執行方式 |
| 操作或文件 | `README.md` | 本索引（若路徑或職責改變） |

## 精準搜尋

```powershell
# 列出 Go 原始碼，不碰題目框架
rg --files cmd internal migrations api deploy

# 找 HTTP route
rg 'Handle|HandleFunc|METHOD|/api/' internal/adapter/httpapi api

# 找 submission 狀態的使用處
rg 'Status(Pending|Running|AC|WA|SE|CE|RE|TLE)|SubmissionStatus' internal migrations

# 找設定值
rg 'HTTP_ADDR|DATABASE_URL|JWT_|PROBLEMS_ROOT|UPLOAD_ROOT|BUILD_ROOT|JUDGE_' internal .env.example deploy

# 只搜尋某一道題
rg '<關鍵字>' test-framework/113final006
```

## 修改原則

- 所有 go 程式都加上簡單的中文註解，每個 funtion 都簡單標明用途。
- 依賴方向維持 `adapter -> usecase -> domain`；`domain` 不 import HTTP、SQL、Docker 或檔案系統實作。
- 外部系統的介面放在 `internal/usecase/ports.go`，具體實作放在對應的 `internal/adapter/`。
- API 的組裝只放在 `internal/app/`，執行檔入口保持精簡。
- DB schema 有變更時新增 migration，不覆寫已部署的 migration。
- `.env.example` 只放安全範例；真實 `.env`、私鑰、上傳內容、build 與 log 不提交。
- 新增、移動或改變主要檔案職責時，同步更新本索引。

## 基本驗證

```powershell
go fmt ./...
go test ./...
go vet ./...
```

修改 CMake 題目框架時，另依 `test-framework/README.md` 執行該題的 configure、build 與 CTest。

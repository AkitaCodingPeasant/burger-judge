# REGS 線上評測後端

REGS 提供帳號、題目、提交、統計與 Docker 判題 API。API 使用 JWT/RBAC；PostgreSQL 保存資料，題目 ZIP、提交原始碼與判題 log 保存於本機 `data/`。

## 環境需求

- Go 1.23 以上
- Docker Desktop（必須使用 Linux containers）
- `docker-compose` 或 `docker compose`
- 判題 image：`yhlib/cs3060701`

## 第一次啟動

```powershell
$env:ADMIN_ACCOUNT = "admin"
$env:ADMIN_PASSWORD = "請換成至少八個字元的密碼"
docker compose up --build -d
```

API 預設位於 `http://localhost:8080`。Swagger UI 位於 `http://localhost:8080/docs`，並從同源的 `/openapi.yaml` 載入規格；第一次開啟頁面時需要網路連線以下載 Swagger UI 前端資源。啟動時會自動：

1. 連線 PostgreSQL 並執行 `migrations/*.up.sql`。
2. 產生 `secrets/private.pem` 與 `secrets/public.pem`（若尚不存在）。
3. 將 `test-framework/113final*` 複製至本機 `data/problems/` 並登錄題目。
4. 依 `ADMIN_ACCOUNT`、`ADMIN_PASSWORD` 建立或更新系統 Admin。

`.env.example` 是設定範本；Go 不會自動載入該檔案，執行前應將需要的值設成環境變數。真實密碼、`.env`、PEM 金鑰與 `data/` 不可提交 Git。

若要直接在本機開發 API，可只執行 `docker compose up -d postgres`，設定管理員環境變數後再執行 `go run ./cmd/api`。

## 快速檢查

```powershell
Invoke-RestMethod http://localhost:8080/healthz

$body = @{ account = "student01"; password = "student-password" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/users/register -ContentType application/json -Body $body
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/users/login -ContentType application/json -Body $body
$headers = @{ Authorization = "Bearer $($login.token)" }
Invoke-RestMethod -Uri http://localhost:8080/api/users/me -Headers $headers
Invoke-RestMethod -Uri http://localhost:8080/api/problems
```

提交程式與上傳題目使用 `multipart/form-data`，建議用 curl：

```powershell
curl.exe -X POST http://localhost:8080/api/submissions `
  -H "Authorization: Bearer $($login.token)" `
  -F "problemId=113final001" `
  -F "source=@submission.zip;type=application/zip"
```

完整欄位、回應與權限請看 [OpenAPI](api/openapi.yaml)。現場展示可直接依照 [Demo 教學](docs/demo-guide.md)。基本及進階判題測試請看 [basic20-testing](docs/basic20-testing.md) 與 [advanced10-testing](docs/advanced10-testing.md)。資料模型請看 [ERD](docs/erd.md)。

## 開發驗證

```powershell
go fmt ./...
go test ./...
go vet ./...
```

停止整套後端但保留資料：

```powershell
docker compose down
```

只有確定要清除所有資料時才執行 `docker compose down -v`。

## 使用 Compose 啟動整套後端

以下指令會建立並啟動 PostgreSQL、Go API，以及 API 容器內的判題 Docker daemon：

```powershell
$env:ADMIN_PASSWORD = "請換成安全密碼"
docker compose up --build -d
docker compose ps
Invoke-RestMethod http://localhost:8080/healthz
```

可用 `docker compose logs -f api` 查看 API 與判題 daemon 的日誌。停止服務時，`docker compose down` 會保留 PostgreSQL、JWT 金鑰、上傳內容與判題 image cache；加上 `-v` 才會一併清除。

API 使用 Docker-in-Docker，因此 `privileged: true` 僅適合本機作業或可信環境，不應直接用於公開的正式環境。

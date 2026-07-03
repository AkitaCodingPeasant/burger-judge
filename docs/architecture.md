# REGS Go 專案架構

## 架構決策

採用「模組化單體 + 獨立背景 Worker」。API 與 Worker 共用 domain、use case 與 adapter，部署時則是兩個 process。第一版不引入 Redis；PostgreSQL 的 submissions 表就是持久化 queue，Worker 以 `FOR UPDATE SKIP LOCKED` 搶工作，當機後也不會遺失任務。

```text
Client
  │ HTTP / JWT
  ▼
cmd/api ──► handler/middleware ──► usecase ──► PostgreSQL / file storage
                                             │
                                      PENDING submission
                                             │
cmd/worker ──► queue poller ──► judge service┴──► Docker ─► CMake ─► CTest
                                      │
                                      └────────► logs / result / score
```

## 目錄

```text
.
├── cmd/
│   ├── api/                 # HTTP server 入口
│   └── worker/              # 非同步判題入口
├── internal/
│   ├── app/                 # dependency wiring、process lifecycle
│   ├── config/              # 環境變數設定
│   ├── domain/              # User、Problem、Submission 與狀態規則
│   ├── usecase/             # application service 與 ports/interfaces
│   └── adapter/
│       ├── httpapi/         # handler、JWT、RBAC、request/response DTO
│       ├── postgres/        # repository 與 queue claim
│       ├── filestorage/     # ZIP、題目、log 實體檔
│       └── dockerjudge/     # Docker/CMake/CTest 實作
├── migrations/              # PostgreSQL schema migration
├── api/openapi.yaml         # OpenAPI 3.0
├── docs/                    # 架構、ERD、操作文件
├── deploy/                  # Dockerfile、compose
└── data/                    # local runtime data；不進版控
```

依賴方向固定為 `adapter → usecase → domain`。`domain` 不 import HTTP、SQL 或 Docker 套件；外部技術都由 `usecase` 中的 interface 隔離。

## 判題狀態機

```text
PENDING → RUNNING → AC | WA | SE | CE | RE | TLE
```

- 上傳完成且資料列建立成功後，API 立刻回 `202 Accepted` 與 `operatorId`。
- Worker claim 任務時，在同一個 transaction 將狀態改成 `RUNNING`。
- Configure 失敗為 `SE`；build 失敗為 `CE`；執行 exit code 非 0 為 `RE`；超時為 `TLE`；成功後再比對 expected 決定 `AC/WA`。
- 每一階段分別保存 `configure.log`、`compile.log`、`output.log`。

## 判題安全邊界

- ZIP 解壓縮前限制檔案數、總大小與單檔大小，並拒絕 `..`、絕對路徑與 symlink（Zip Slip）。
- Configure/build 與 run 使用不同容器；run 設定無網路、唯讀 root filesystem、非 root user、CPU/記憶體/PID 限制。
- Host 只掛載該 submission 的隨機 workspace，不掛 Docker socket、題庫根目錄或金鑰。
- Go `context.WithTimeout` 負責主機端 deadline；逾時後強制移除容器，不能只相信容器內計時。
- 上傳 ZIP 與 log 放檔案系統，DB 只存 metadata/path，避免資料庫快速膨脹。

## RBAC

Guest 不是資料庫角色，而是「沒有 JWT 的 request」。JWT 驗證後把 `user_id` 與 `role` 放入 request context；middleware 只做認證與角色門檻，handler/usecase 還要檢查資源所有權。例如一般 User 只能讀自己的 source 與 submission detail，Admin 才能讀別人的內容。

資料庫連線帳號只給應用程式需要的 CRUD 權限，不讓 HTTP 傳入任意 SQL，也不把 Admin 直接映射成 PostgreSQL superuser。

## 建議實作順序

1. PostgreSQL migration、註冊/登入、ES256 JWT 與 RBAC middleware。
2. Problem CRUD、題目 ZIP/settings.yaml 儲存。
3. Submission upload：安全解壓縮、建立 `PENDING`、立即回 operatorId。
4. Worker queue claim 與 semaphore；先跑假的 JudgeRunner 驗證狀態流。
5. Docker configure/build/run、timeout、log 與 expected 比對。
6. 查詢/統計 API、OpenAPI、ERD、整合測試。

## 建議套件（真正用到時再加入）

- Router：Go 1.22+ `net/http` 即可；路由變複雜時再採 `chi`。
- PostgreSQL：`pgx/v5`；migration：`golang-migrate`。
- JWT：`golang-jwt/jwt/v5`；密碼：`golang.org/x/crypto/bcrypt`。
- Docker：官方 Docker Engine Go SDK；YAML：`gopkg.in/yaml.v3`。
- 測試：標準 `testing` + `httptest`，需要容器整合測試時再加 Testcontainers。


# REGS 專案 Demo 教學

這份文件以 Windows PowerShell 示範完整流程，預計約 10～15 分鐘。Demo 會涵蓋 PostgreSQL、JWT/RBAC、題目管理、非同步 Queue、Docker/CMake/CTest 判題、log、原始碼下載及統計 API。

## 一、Demo 前準備

確認已安裝並啟動：

- Go 1.23 以上
- Docker Desktop（使用 Linux containers）
- `postgres:17` image
- `yhlib/cs3060701` 判題 image

在專案根目錄執行：

```powershell
go version
docker version
docker image inspect postgres:17
docker image inspect yhlib/cs3060701
go test ./...
```

如果缺少 image，只需下載一次：

```powershell
docker pull postgres:17
docker pull yhlib/cs3060701
```

## 二、啟動 PostgreSQL

開啟第一個 PowerShell，切換到專案根目錄：

```powershell
cd D:\NTUST\11402\Backend\Project
docker-compose up -d postgres
docker-compose ps
```

預期 PostgreSQL container 顯示為 `Up` 或 `healthy`。

## 三、設定 Admin 並啟動 API

在同一個 PowerShell 設定答題系統的 Admin 帳密：

```powershell
$env:ADMIN_ACCOUNT = "admin"
$env:ADMIN_PASSWORD = "admin-password"
go run ./cmd/api
```

`ADMIN_PASSWORD` 至少需要 8 個字元。第一次啟動時，系統會：

1. 自動執行 PostgreSQL migration。
2. 產生 JWT ES256 金鑰。
3. 載入 `test-framework/113final*` 五道題目。
4. 建立或更新 Admin 帳號。
5. 在 `http://localhost:8080` 啟動 API。

這個 PowerShell 必須保持開啟。

## 四、Health check

開啟第二個 PowerShell並切換到專案根目錄：

```powershell
cd D:\NTUST\11402\Backend\Project
$base = "http://localhost:8080"
Invoke-RestMethod "$base/healthz"
```

預期結果：

```text
status
------
ok
```

## 五、示範 Admin 登入

```powershell
$adminBody = @{
  account  = "admin"
  password = "admin-password"
} | ConvertTo-Json

$adminLogin = Invoke-RestMethod `
  -Method Post `
  -Uri "$base/api/users/login" `
  -ContentType "application/json" `
  -Body $adminBody

$adminToken = $adminLogin.token
$adminHeaders = @{ Authorization = "Bearer $adminToken" }
$adminLogin.user | Format-List
```

預期 `role` 為 `admin`。JWT 之後會放在 `Authorization: Bearer <token>`。

## 六、示範使用者註冊、登入與個人資料

每次 Demo 使用不同帳號，避免重複註冊：

```powershell
$studentAccount = "demo_$(Get-Date -Format 'MMddHHmmss')"
$studentBody = @{
  account  = $studentAccount
  password = "student-password"
} | ConvertTo-Json

Invoke-RestMethod `
  -Method Post `
  -Uri "$base/api/users/register" `
  -ContentType "application/json" `
  -Body $studentBody

$studentLogin = Invoke-RestMethod `
  -Method Post `
  -Uri "$base/api/users/login" `
  -ContentType "application/json" `
  -Body $studentBody

$studentToken = $studentLogin.token
$studentHeaders = @{ Authorization = "Bearer $studentToken" }
$studentID = $studentLogin.user.id

Invoke-RestMethod "$base/api/users/me" -Headers $studentHeaders | Format-List
```

預期角色為 `user`。

## 七、示範 Guest 題目查詢

題目列表與單題資料不需要 JWT：

```powershell
Invoke-RestMethod "$base/api/problems" | Format-Table id,title
Invoke-RestMethod "$base/api/problems/113final001" | Format-List
```

預期列表包含五道 `113final*` 題目。

## 八、示範 RBAC 權限

下載完整題目 ZIP 需要 Admin：

```powershell
New-Item -ItemType Directory -Force data\demo | Out-Null

curl.exe -i `
  "$base/api/problems/113final001/testcases" `
  -H "Authorization: Bearer $studentToken"
```

一般 User 預期得到 `403 Forbidden`。

改用 Admin：

```powershell
curl.exe -sS `
  "$base/api/problems/113final001/testcases" `
  -H "Authorization: Bearer $adminToken" `
  -o data\demo\113final001-problem.zip

Get-Item data\demo\113final001-problem.zip
```

預期 ZIP 成功下載且檔案大小大於 0。

## 九、準備正確答案 ZIP

將官方 solution 壓縮成學生 submission。ZIP 根目錄必須直接包含 `CMakeLists.txt`：

```powershell
Compress-Archive `
  -Path test-framework\113final001\solution\* `
  -DestinationPath data\demo\113final001-ac.zip `
  -Force
```

## 十、提交並示範非同步 Queue

```powershell
$acceptedJSON = curl.exe -sS -X POST "$base/api/submissions" `
  -H "Authorization: Bearer $studentToken" `
  -F "problemId=113final001" `
  -F "source=@data/demo/113final001-ac.zip;type=application/zip"

$accepted = $acceptedJSON | ConvertFrom-Json
$operatorId = $accepted.operatorId
$accepted | Format-List
```

API 應立即回傳：

```text
operatorId : 一組 UUID
status     : PENDING
```

這代表 submission 已進入背景 Queue，HTTP 不會等待 Docker 判題完成。

## 十一、輪詢判題結果

```powershell
do {
  Start-Sleep -Seconds 1
  $result = Invoke-RestMethod `
    "$base/api/submissions/$operatorId" `
    -Headers $studentHeaders

  Write-Host "status=$($result.status), score=$($result.score)"
} while ($result.status -in @("PENDING", "RUNNING"))

$result | Format-List
```

正確 solution 預期得到：

```text
status : AC
score  : 100
```

這段流程實際執行 Docker、CMake configure、build 與 CTest。

## 十二、查看三段判題 Log

```powershell
curl.exe "$base/api/submissions/$operatorId/logs/configure" `
  -H "Authorization: Bearer $studentToken"

curl.exe "$base/api/submissions/$operatorId/logs/compile" `
  -H "Authorization: Bearer $studentToken"

curl.exe "$base/api/submissions/$operatorId/logs/run" `
  -H "Authorization: Bearer $studentToken"
```

三段 log 的實體檔位於：

```text
data/uploads/<operatorId>/logs/
```

## 十三、下載 Submission 原始碼

```powershell
curl.exe -sS `
  "$base/api/submissions/$operatorId/source" `
  -H "Authorization: Bearer $studentToken" `
  -o data\demo\downloaded-source.zip

Get-Item data\demo\downloaded-source.zip
```

## 十四、示範提交紀錄與統計

```powershell
Invoke-RestMethod "$base/api/submissions" -Headers $studentHeaders | Format-Table
Invoke-RestMethod "$base/api/users/$studentID/submissions" | Format-Table
Invoke-RestMethod "$base/api/stats/problems/113final001" | Format-List
Invoke-RestMethod "$base/api/stats/users/$studentID" | Format-List
```

題目統計應至少有一筆 submission 與一筆 accepted；使用者統計的 solved 應至少為 1。

## 十五、示範 Rerun

```powershell
$rerun = Invoke-RestMethod `
  -Method Post `
  -Uri "$base/api/submissions/$operatorId/rerun" `
  -Headers $studentHeaders

$rerun | Format-List
```

預期使用相同的 operatorId，狀態重新變成 `PENDING`。

## 十六、示範 Admin 題目 CRUD

使用前面建立的 ZIP 新增一題：

```powershell
$createdJSON = curl.exe -sS -X PUT "$base/api/problems" `
  -H "Authorization: Bearer $adminToken" `
  -F "problemId=demo001" `
  -F "title=Demo Problem" `
  -F "description=Temporary problem for project demo" `
  -F "archive=@data/demo/113final001-ac.zip;type=application/zip"

$createdJSON | ConvertFrom-Json | Format-List
Invoke-RestMethod "$base/api/problems/demo001" | Format-List
```

Demo 完成後刪除臨時題目：

```powershell
Invoke-RestMethod `
  -Method Delete `
  -Uri "$base/api/problems/demo001" `
  -Headers $adminHeaders
```

刪除成功回傳 HTTP `204 No Content`，PowerShell 通常不會顯示內容。

## 十七、示範 Logout

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri "$base/api/users/logout" `
  -Headers $studentHeaders
```

再次使用舊 token：

```powershell
try {
  Invoke-RestMethod "$base/api/users/me" -Headers $studentHeaders
} catch {
  $_.Exception.Response.StatusCode.value__
}
```

預期得到 `401`，表示 JWT 已撤銷。

## 十八、結束 Demo

回到執行 `go run ./cmd/api` 的第一個 PowerShell，按 `Ctrl+C` 停止 API。

保留 PostgreSQL 資料：

```powershell
docker-compose down
```

若確定要清除所有帳號、題目 metadata 與 submission 資料：

```powershell
docker-compose down -v
```

`-v` 會永久刪除 PostgreSQL volume，平常不要使用。

## 常見問題

### API 無法啟動

先確認 PostgreSQL：

```powershell
docker-compose ps
Get-Content data\api.stderr.log -ErrorAction SilentlyContinue
```

### Port 8080 已被使用

```powershell
Get-NetTCPConnection -LocalPort 8080
```

停止原本的 API，或設定其他 port：

```powershell
$env:HTTP_ADDR = ":8081"
go run ./cmd/api
```

### Submission 一直是 PENDING 或得到 SE

確認 Docker Desktop 正在執行，並確認判題 image：

```powershell
docker version
docker image inspect yhlib/cs3060701
```

### 得到 401

確認 Header 使用目前登入取得的 token：

```text
Authorization: Bearer <JWT>
```

### 得到 403

代表已登入，但角色不足，通常是 User 嘗試操作 Admin API。

## Demo 評分重點對照

| 評分內容 | Demo 章節 |
|---|---|
| ZIP 上傳與解壓縮 | 九、十 |
| Docker/CMake/CTest | 十、十一 |
| 非同步 Queue | 十、十一 |
| AC 與分數 | 十一 |
| 三段 log | 十二 |
| Rerun | 十五 |
| JWT 與 RBAC | 五、六、八、十七 |
| 題目 API | 七、八、十六 |
| Submission API | 十～十五 |
| 統計 API | 十四 |
| OpenAPI、ERD、操作文件 | `api/openapi.yaml`、`docs/erd.md`、`README.md` |

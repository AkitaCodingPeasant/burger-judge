# 進階 10 分測試

進階項目包含：非同步 Job Queue、同一 Job 重跑，以及 Semaphore 最大併發數限制。

## 自動測試

測試 Queue、rerun 與 Semaphore：

```powershell
go test ./internal/usecase -run TestSubmissionService -v -count=1
```

測試正式 JWT/RBAC Router 的 rerun API：

```powershell
go test ./internal/adapter/httpapi -run TestFullRouterRerunAndLog -v -count=1
```

執行包含真實 Docker、CMake、CTest 與 rerun 的整合測試：

```powershell
$env:REGS_DOCKER_INTEGRATION = "1"
go test ./internal/app -run TestBasic20Pipeline -v -count=1
```

## 手動測試

先依 README 啟動 PostgreSQL 與 API，接著註冊並登入：

```powershell
$body = @{ account = "queue-test"; password = "queue-password" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/users/register -ContentType application/json -Body $body
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/users/login -ContentType application/json -Body $body
$token = $login.token
```

準備正確答案 ZIP：

```powershell
New-Item -ItemType Directory -Force data\demo
Compress-Archive -Path test-framework\113final006\solution\* -DestinationPath data\demo\113final006-ac.zip -Force
```

連續提交三次：

```powershell
$operatorIds = 1..3 | ForEach-Object {
  $result = curl.exe -sS -X POST http://localhost:8080/api/submissions `
    -H "Authorization: Bearer $token" `
    -F "problemId=113final006" `
    -F "source=@data/demo/113final006-ac.zip;type=application/zip" | ConvertFrom-Json
  $result.operatorId
}
$operatorIds
```

當 `MAX_JUDGE_WORKERS=2` 時，最多兩個 Job 同時為 `RUNNING`，其餘維持 `PENDING`。完成後可使用相同 operatorId 重跑：

```powershell
$operatorId = $operatorIds[0]
curl.exe -i -X POST "http://localhost:8080/api/submissions/$operatorId/rerun" `
  -H "Authorization: Bearer $token"
```

預期回傳 HTTP `202` 與狀態 `PENDING`。Queue 與 Semaphore 位於 API 行程內；PostgreSQL 持久化 submission 資料與最終狀態。

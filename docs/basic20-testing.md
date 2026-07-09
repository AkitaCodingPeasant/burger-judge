# 基本 20 分測試

基本項目包含 ZIP 解壓縮、Docker 網路隔離、CMake configure/build、AC/WA/CE/RE 狀態與三段 log。

## 環境確認

```powershell
go version
docker version
docker image inspect yhlib/cs3060701
```

若尚未下載判題 image：

```powershell
docker pull yhlib/cs3060701
```

## 一般 Go 測試

```powershell
go test ./...
go vet ./...
gofmt -l cmd internal
```

`gofmt -l` 正常時不會輸出檔名。

## 真實 Docker 判題測試

在目前 PowerShell 設定：

```powershell
$env:REGS_DOCKER_INTEGRATION = "1"
```

驗證所有官方 solution：

```powershell
go test ./internal/adapter/dockerjudge -run TestRunnerWithOfficialSolutions -v -count=1
```

驗證 AC、WA、CE、RE 與 timeout：

```powershell
go test ./internal/adapter/dockerjudge -run TestRunnerClassifiesRealPrograms -v -count=1
```

驗證 HTTP 上傳、非同步判題、結果、三段 log 與 rerun：

```powershell
go test ./internal/app -run TestBasic20Pipeline -v -count=1
```

## 完整 API 手動測試

先依 README 啟動 PostgreSQL 與 API，註冊並登入：

```powershell
$body = @{ account = "basic-test"; password = "basic-password" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/users/register -ContentType application/json -Body $body
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/users/login -ContentType application/json -Body $body
$token = $login.token
```

建立 submission ZIP：

```powershell
New-Item -ItemType Directory -Force data\demo
Compress-Archive -Path test-framework\113final001\solution\* -DestinationPath data\demo\113final001-ac.zip -Force
```

提交並取得 operatorId：

```powershell
$accepted = curl.exe -sS -X POST http://localhost:8080/api/submissions `
  -H "Authorization: Bearer $token" `
  -F "problemId=113final001" `
  -F "source=@data/demo/113final001-ac.zip;type=application/zip" | ConvertFrom-Json
$operatorId = $accepted.operatorId
```

查詢結果與三段 log：

```powershell
curl.exe -H "Authorization: Bearer $token" "http://localhost:8080/api/submissions/$operatorId"
curl.exe -H "Authorization: Bearer $token" "http://localhost:8080/api/submissions/$operatorId/logs/configure"
curl.exe -H "Authorization: Bearer $token" "http://localhost:8080/api/submissions/$operatorId/logs/compile"
curl.exe -H "Authorization: Bearer $token" "http://localhost:8080/api/submissions/$operatorId/logs/run"
```

正確 solution 最終應得到 `AC` 與分數 `100`。log 實體檔位於 `data/uploads/<operatorId>/logs/`。

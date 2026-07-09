package main

// 此檔案是 API 服務的程式進入點。

import (
	"log"

	"regs/internal/app"
	"regs/internal/config"
)

// main 載入設定並啟動 API 服務。
func main() {
	cfg := config.Load()
	if err := app.RunAPI(cfg); err != nil {
		log.Fatal(err)
	}
}

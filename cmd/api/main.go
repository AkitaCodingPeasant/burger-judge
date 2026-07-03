package main

import (
	"log"

	"regs/internal/app"
	"regs/internal/config"
)

func main() {
	cfg := config.Load()
	if err := app.RunAPI(cfg); err != nil {
		log.Fatal(err)
	}
}


package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	JWTPrivateKey   string
	JWTPublicKey    string
	ProblemsRoot    string
	UploadRoot      string
	BuildRoot       string
	MaxJudgeWorkers int
	JudgeTimeout    time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://regs:regs@localhost:5432/regs?sslmode=disable"),
		JWTPrivateKey:   env("JWT_PRIVATE_KEY", "./secrets/private.pem"),
		JWTPublicKey:    env("JWT_PUBLIC_KEY", "./secrets/public.pem"),
		ProblemsRoot:    env("PROBLEMS_ROOT", "./data/problems"),
		UploadRoot:      env("UPLOAD_ROOT", "./data/uploads"),
		BuildRoot:       env("BUILD_ROOT", "./data/builds"),
		MaxJudgeWorkers: envInt("MAX_JUDGE_WORKERS", 2),
		JudgeTimeout:    time.Duration(envInt("JUDGE_TIMEOUT_MS", 3000)) * time.Millisecond,
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}


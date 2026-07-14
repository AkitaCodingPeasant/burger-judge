package config

import (
	"os"
	"strconv"
	"time"
)

// Config 保存 API、資料庫與判題流程的執行階段設定。
type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	MigrationDir      string
	OpenAPISpecPath   string
	JWTPrivateKey     string
	JWTPublicKey      string
	JWTTTL            time.Duration
	AdminAccount      string
	AdminPassword     string
	ProblemFixtures   string
	ProblemsRoot      string
	UploadRoot        string
	BuildRoot         string
	JudgeImage        string
	MaxJudgeWorkers   int
	JudgeBuildTimeout time.Duration
	JudgeTimeout      time.Duration
}

// Load 讀取環境變數，未設定時使用安全的本機預設值。
func Load() Config {
	return Config{
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		DatabaseURL:       env("DATABASE_URL", "postgres://regs:regs@localhost:5432/regs?sslmode=disable"),
		MigrationDir:      env("MIGRATION_DIR", "./migrations"),
		OpenAPISpecPath:   env("OPENAPI_SPEC_PATH", "./api/openapi.yaml"),
		JWTPrivateKey:     env("JWT_PRIVATE_KEY", "./secrets/private.pem"),
		JWTPublicKey:      env("JWT_PUBLIC_KEY", "./secrets/public.pem"),
		JWTTTL:            time.Duration(envInt("JWT_TTL_MINUTES", 60)) * time.Minute,
		AdminAccount:      env("ADMIN_ACCOUNT", ""),
		AdminPassword:     env("ADMIN_PASSWORD", ""),
		ProblemFixtures:   env("PROBLEM_FIXTURES_ROOT", "./test-framework"),
		ProblemsRoot:      env("PROBLEMS_ROOT", "./data/problems"),
		UploadRoot:        env("UPLOAD_ROOT", "./data/uploads"),
		BuildRoot:         env("BUILD_ROOT", "./data/builds"),
		JudgeImage:        env("JUDGE_IMAGE", "yhlib/cs3060701"),
		MaxJudgeWorkers:   envInt("MAX_JUDGE_WORKERS", 2),
		JudgeBuildTimeout: time.Duration(envInt("JUDGE_BUILD_TIMEOUT_MS", 120000)) * time.Millisecond,
		JudgeTimeout:      time.Duration(envInt("JUDGE_TIMEOUT_MS", 3000)) * time.Millisecond,
	}
}

// env 讀取字串環境變數，空值時回傳預設值。
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// envInt 讀取正整數環境變數，格式錯誤時回傳預設值。
func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

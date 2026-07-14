package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"regs/internal/adapter/authjwt"
	"regs/internal/adapter/dockerjudge"
	"regs/internal/adapter/filestorage"
	"regs/internal/adapter/httpapi"
	"regs/internal/adapter/postgres"
	"regs/internal/config"
	"regs/internal/domain"
	"regs/internal/usecase"
)

// RunAPI 組裝 HTTP 與判題依賴，並持續服務直到收到關閉訊號。
func RunAPI(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx, cfg.MigrationDir); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}

	problemStorage := filestorage.NewProblemStore(cfg.ProblemsRoot)
	fixtureIDs, err := problemStorage.SeedFixtures(ctx, cfg.ProblemFixtures)
	if err != nil {
		return fmt.Errorf("seed problem fixtures: %w", err)
	}
	for _, id := range fixtureIDs {
		if _, err := store.UpsertProblem(ctx, domain.Problem{
			ID:           id,
			Title:        id,
			SettingsPath: filepath.Join(cfg.ProblemsRoot, id),
		}); err != nil {
			return fmt.Errorf("seed problem %s: %w", id, err)
		}
	}

	tokenManager, err := authjwt.New(cfg.JWTPrivateKey, cfg.JWTPublicKey, cfg.JWTTTL)
	if err != nil {
		return fmt.Errorf("initialize JWT keys: %w", err)
	}
	authService := usecase.NewAuthService(store, store, tokenManager)
	if err := authService.EnsureAdmin(ctx, cfg.AdminAccount, cfg.AdminPassword); err != nil {
		return fmt.Errorf("initialize administrator: %w", err)
	}

	storage := filestorage.New(cfg.UploadRoot)
	runner := dockerjudge.New(cfg.JudgeImage, cfg.ProblemsRoot, cfg.BuildRoot, cfg.JudgeBuildTimeout, cfg.JudgeTimeout, storage)
	service := usecase.NewSubmissionService(storage, runner, cfg.MaxJudgeWorkers, store)
	defer service.Close()
	problemService := usecase.NewProblemService(store, problemStorage)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewFullRouter(httpapi.FullDependencies{
			Auth:        authService,
			Problems:    problemService,
			Submissions: service,
			Statistics:  store,
			OpenAPISpec: cfg.OpenAPISpecPath,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	baseURL := serverURL(cfg.HTTPAddr)
	log.Printf("後端連線位置: %s", baseURL)
	log.Printf("Swagger 位置: %s/docs", baseURL)
	// 使用獨立 goroutine 啟動 HTTP 服務，讓主流程能同時等待關閉訊號。
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

// serverURL 將 HTTP 監聽位址轉換成可供使用者開啟的網址。
func serverURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

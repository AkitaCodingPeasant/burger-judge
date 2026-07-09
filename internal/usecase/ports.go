package usecase

import (
	"context"
	"errors"
	"io"
	"time"

	"regs/internal/domain"
)

// ErrInvalidSource 表示上傳檔格式錯誤或無法安全解壓縮。
var ErrInvalidSource = errors.New("invalid source archive")

var (
	ErrNotFound = errors.New("resource not found")
	ErrConflict = errors.New("resource already exists")
)

// UserRepository 定義使用者帳號的持久化操作。
type UserRepository interface {
	CreateUser(context.Context, domain.User) error
	GetUserByID(context.Context, string) (domain.User, error)
	GetUserByAccount(context.Context, string) (domain.User, error)
	EnsureAdmin(context.Context, domain.User) error
}

// TokenRepository 保存已登出的 JWT ID，讓 logout 能立即失效。
type TokenRepository interface {
	RevokeToken(context.Context, string, time.Time) error
	IsTokenRevoked(context.Context, string) (bool, error)
}

// TokenManager 簽發並驗證應用程式 JWT。
type TokenManager interface {
	Issue(context.Context, domain.User) (string, error)
	Parse(context.Context, string) (domain.Principal, error)
}

// ProblemRepository 定義題目 CRUD。
type ProblemRepository interface {
	ListProblems(context.Context) ([]domain.Problem, error)
	GetProblem(context.Context, string) (domain.Problem, error)
	UpsertProblem(context.Context, domain.Problem) (domain.Problem, error)
	DeleteProblem(context.Context, string) error
}

// SubmissionRepository 定義提交資料的持久化操作。
type SubmissionRepository interface {
	// Create 建立一筆提交資料。
	Create(context.Context, domain.Submission) error
	// Get 依 ID 取得提交資料。
	Get(context.Context, string) (domain.Submission, error)
	// UpdateStatus 更新指定提交的狀態。
	// UpdateSubmission 保存完整判題狀態、分數與時間。
	UpdateSubmission(context.Context, domain.Submission) error
	// ListSubmissionsByUser 取得指定使用者的提交紀錄。
	ListSubmissionsByUser(context.Context, string) ([]domain.Submission, error)
}

// StatisticsRepository 提供題目與使用者統計查詢。
type StatisticsRepository interface {
	ProblemStatistics(context.Context, string) (domain.ProblemStatistics, error)
	UserStatistics(context.Context, string) (domain.UserStatistics, error)
}

// ArchiveStorage 定義原始碼壓縮檔與階段日誌的儲存操作。
type ArchiveStorage interface {
	// SaveSource 儲存並處理提交的原始碼壓縮檔。
	SaveSource(context.Context, string, io.Reader) error
	// OpenSource 開啟原始上傳檔。
	OpenSource(context.Context, string) (io.ReadCloser, error)
	// SaveLog 儲存指定階段的日誌。
	SaveLog(context.Context, string, string, io.Reader) error
	// OpenLog 開啟指定階段的日誌。
	OpenLog(context.Context, string, string) (io.ReadCloser, error)
}

// ProblemStorage 安全保存、下載與刪除管理者上傳的題目 ZIP。
type ProblemStorage interface {
	SaveProblem(context.Context, string, io.Reader) (string, error)
	OpenProblem(context.Context, string) (io.ReadCloser, error)
	DeleteProblem(context.Context, string) error
}

// JudgeRunner 定義 Docker、CMake 與 CTest 判題流程的介面。
type JudgeRunner interface {
	// Run 執行一次判題並回傳狀態、分數與錯誤。
	Run(context.Context, domain.Submission) (domain.SubmissionStatus, int, error)
}

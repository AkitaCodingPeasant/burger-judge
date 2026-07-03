package usecase

import (
	"context"
	"io"

	"regs/internal/domain"
)

// SubmissionRepository is implemented by the PostgreSQL adapter.
type SubmissionRepository interface {
	Create(context.Context, domain.Submission) error
	Get(context.Context, string) (domain.Submission, error)
	UpdateStatus(context.Context, string, domain.SubmissionStatus) error
	ClaimPending(context.Context) (domain.Submission, error)
}

// ArchiveStorage owns ZIPs and stage logs; handlers never manipulate paths.
type ArchiveStorage interface {
	SaveSource(context.Context, string, io.Reader) error
	OpenSource(context.Context, string) (io.ReadCloser, error)
	SaveLog(context.Context, string, string, io.Reader) error
}

// JudgeRunner is the boundary around Docker/CMake/CTest.
type JudgeRunner interface {
	Run(context.Context, domain.Submission) (domain.SubmissionStatus, int, error)
}


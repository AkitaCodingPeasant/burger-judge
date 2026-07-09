package usecase

import (
	"context"
	"errors"
	"io"
	"strings"

	"regs/internal/domain"
)

var ErrInvalidProblem = errors.New("invalid problem")

// ProblemService 協調題目 metadata 與 ZIP 檔案儲存。
type ProblemService struct {
	repository ProblemRepository
	storage    ProblemStorage
}

func NewProblemService(repository ProblemRepository, storage ProblemStorage) *ProblemService {
	return &ProblemService{repository: repository, storage: storage}
}

func (s *ProblemService) List(ctx context.Context) ([]domain.Problem, error) {
	return s.repository.ListProblems(ctx)
}

func (s *ProblemService) Get(ctx context.Context, id string) (domain.Problem, error) {
	return s.repository.GetProblem(ctx, id)
}

func (s *ProblemService) Upsert(ctx context.Context, id, title, description string, archive io.Reader) (domain.Problem, error) {
	id = strings.TrimSpace(id)
	title = strings.TrimSpace(title)
	if id == "" || title == "" || archive == nil {
		return domain.Problem{}, ErrInvalidProblem
	}
	settingsPath, err := s.storage.SaveProblem(ctx, id, archive)
	if err != nil {
		return domain.Problem{}, err
	}
	return s.repository.UpsertProblem(ctx, domain.Problem{
		ID:           id,
		Title:        title,
		Description:  strings.TrimSpace(description),
		SettingsPath: settingsPath,
	})
}

func (s *ProblemService) Delete(ctx context.Context, id string) error {
	if err := s.repository.DeleteProblem(ctx, id); err != nil {
		return err
	}
	return s.storage.DeleteProblem(ctx, id)
}

func (s *ProblemService) OpenArchive(ctx context.Context, id string) (io.ReadCloser, error) {
	if _, err := s.repository.GetProblem(ctx, id); err != nil {
		return nil, err
	}
	return s.storage.OpenProblem(ctx, id)
}

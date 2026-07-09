package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"regs/internal/domain"
	"regs/internal/usecase"
)

// Store 實作所有 PostgreSQL repositories。
type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Migrate 依檔名順序執行尚未套用的 *.up.sql。
func (s *Store) Migrate(ctx context.Context, directory string) error {
	if _, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&applied)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO users(id,account,password_hash,role) VALUES($1,$2,$3,$4)`, user.ID, user.Account, user.PasswordHash, user.Role)
	return mapError(err)
}

func (s *Store) EnsureAdmin(ctx context.Context, user domain.User) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO users(id,account,password_hash,role) VALUES($1,$2,$3,'admin')
ON CONFLICT(account) DO UPDATE SET password_hash=EXCLUDED.password_hash, role='admin'`, user.ID, user.Account, user.PasswordHash)
	return err
}

func (s *Store) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT id,account,password_hash,role,created_at FROM users WHERE id=$1`, id))
}

func (s *Store) GetUserByAccount(ctx context.Context, account string) (domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT id,account,password_hash,role,created_at FROM users WHERE account=$1`, account))
}

func scanUser(row pgx.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Account, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return user, mapError(err)
}

func (s *Store) RevokeToken(ctx context.Context, id string, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO revoked_tokens(token_id,expires_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, expires)
	return err
}

func (s *Store) IsTokenRevoked(ctx context.Context, id string) (bool, error) {
	var revoked bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE token_id=$1 AND expires_at > now())`, id).Scan(&revoked)
	return revoked, err
}

func (s *Store) ListProblems(ctx context.Context) ([]domain.Problem, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,title,description,settings_path,created_at,updated_at FROM problems ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []domain.Problem
	for rows.Next() {
		value, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) GetProblem(ctx context.Context, id string) (domain.Problem, error) {
	return scanProblem(s.pool.QueryRow(ctx, `SELECT id,title,description,settings_path,created_at,updated_at FROM problems WHERE id=$1`, id))
}

func (s *Store) UpsertProblem(ctx context.Context, problem domain.Problem) (domain.Problem, error) {
	row := s.pool.QueryRow(ctx, `
INSERT INTO problems(id,title,description,settings_path) VALUES($1,$2,$3,$4)
ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,description=EXCLUDED.description,settings_path=EXCLUDED.settings_path,updated_at=now()
RETURNING id,title,description,settings_path,created_at,updated_at`, problem.ID, problem.Title, problem.Description, problem.SettingsPath)
	return scanProblem(row)
}

func (s *Store) DeleteProblem(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM problems WHERE id=$1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return usecase.ErrConflict
		}
		return err
	}
	if result.RowsAffected() == 0 {
		return usecase.ErrNotFound
	}
	return nil
}

func scanProblem(row pgx.Row) (domain.Problem, error) {
	var problem domain.Problem
	err := row.Scan(&problem.ID, &problem.Title, &problem.Description, &problem.SettingsPath, &problem.CreatedAt, &problem.UpdatedAt)
	return problem, mapError(err)
}

func (s *Store) Create(ctx context.Context, submission domain.Submission) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO submissions(id,user_id,problem_id,status,score,source_path,created_at,started_at,ended_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, submission.ID, submission.UserID, submission.ProblemID, submission.Status, submission.Score, submission.ID, submission.CreatedAt, submission.StartedAt, submission.EndedAt)
	return mapError(err)
}

func (s *Store) Get(ctx context.Context, id string) (domain.Submission, error) {
	return scanSubmission(s.pool.QueryRow(ctx, `SELECT id,user_id,problem_id,status,score,created_at,started_at,ended_at FROM submissions WHERE id=$1`, id))
}

func (s *Store) UpdateSubmission(ctx context.Context, submission domain.Submission) error {
	result, err := s.pool.Exec(ctx, `UPDATE submissions SET status=$2,score=$3,started_at=$4,ended_at=$5 WHERE id=$1`, submission.ID, submission.Status, submission.Score, submission.StartedAt, submission.EndedAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return usecase.ErrNotFound
	}
	return nil
}

func (s *Store) ListSubmissionsByUser(ctx context.Context, userID string) ([]domain.Submission, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,user_id,problem_id,status,score,created_at,started_at,ended_at FROM submissions WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []domain.Submission
	for rows.Next() {
		value, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func scanSubmission(row pgx.Row) (domain.Submission, error) {
	var value domain.Submission
	err := row.Scan(&value.ID, &value.UserID, &value.ProblemID, &value.Status, &value.Score, &value.CreatedAt, &value.StartedAt, &value.EndedAt)
	return value, mapError(err)
}

func (s *Store) ProblemStatistics(ctx context.Context, problemID string) (domain.ProblemStatistics, error) {
	var value domain.ProblemStatistics
	value.ProblemID = problemID
	err := s.pool.QueryRow(ctx, `
SELECT count(*),count(*) FILTER(WHERE status='AC'),count(DISTINCT user_id)
FROM submissions WHERE problem_id=$1`, problemID).Scan(&value.Submissions, &value.Accepted, &value.Users)
	return value, err
}

func (s *Store) UserStatistics(ctx context.Context, userID string) (domain.UserStatistics, error) {
	var value domain.UserStatistics
	value.UserID = userID
	err := s.pool.QueryRow(ctx, `
SELECT count(*),count(*) FILTER(WHERE status='AC'),count(DISTINCT problem_id) FILTER(WHERE status='AC')
FROM submissions WHERE user_id=$1`, userID).Scan(&value.Submissions, &value.Accepted, &value.Solved)
	return value, err
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return usecase.ErrConflict
	}
	return err
}

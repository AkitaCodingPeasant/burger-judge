package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"regs/internal/domain"
)

var (
	// ErrSubmissionNotFound 表示找不到指定的提交 ID。
	ErrSubmissionNotFound = errors.New("submission not found")
	// ErrServiceStopped 表示判題服務已停止接收工作。
	ErrServiceStopped = errors.New("judge service is stopped")
	// ErrSubmissionNotRerunnable 表示工作尚未完成，不能重複排入佇列。
	ErrSubmissionNotRerunnable = errors.New("submission is pending or running")
)

// SubmissionService 接收提交，以 FIFO Job Queue 排程並限制同時判題數量。
type SubmissionService struct {
	storage    ArchiveStorage
	runner     JudgeRunner
	repository SubmissionRepository
	ctx        context.Context
	cancel     context.CancelFunc

	queueMu sync.Mutex
	queue   []string
	queueCV *sync.Cond
	stopped bool

	// semaphore 明確限制同時執行 JudgeRunner 的工作數量。
	semaphore    chan struct{}
	dispatcherWG sync.WaitGroup
	jobsWG       sync.WaitGroup

	mu          sync.RWMutex
	submissions map[string]domain.Submission
}

// NewSubmissionService 建立非同步 Job Queue 與指定容量的 semaphore。
func NewSubmissionService(storage ArchiveStorage, runner JudgeRunner, workers int, repositories ...SubmissionRepository) *SubmissionService {
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &SubmissionService{
		storage:     storage,
		runner:      runner,
		ctx:         ctx,
		cancel:      cancel,
		semaphore:   make(chan struct{}, workers),
		submissions: make(map[string]domain.Submission),
	}
	if len(repositories) > 0 {
		s.repository = repositories[0]
	}
	s.queueCV = sync.NewCond(&s.queueMu)
	s.dispatcherWG.Add(1)
	go s.dispatch()
	return s
}

// Submit 驗證並儲存上傳檔，再將提交立即加入非同步判題佇列。
func (s *SubmissionService) Submit(ctx context.Context, problemID string, source io.Reader) (domain.Submission, error) {
	return s.SubmitForUser(ctx, "", problemID, source)
}

// SubmitForUser 建立具擁有者的持久化提交，供需要 JWT 的正式 API 使用。
func (s *SubmissionService) SubmitForUser(ctx context.Context, userID, problemID string, source io.Reader) (domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return domain.Submission{}, err
	}
	if err := s.ctx.Err(); err != nil {
		return domain.Submission{}, ErrServiceStopped
	}
	if problemID == "" {
		return domain.Submission{}, errors.New("problemId is required")
	}
	id, err := newID()
	if err != nil {
		return domain.Submission{}, err
	}
	if err := s.storage.SaveSource(ctx, id, source); err != nil {
		return domain.Submission{}, err
	}
	submission := domain.Submission{
		ID:        id,
		UserID:    userID,
		ProblemID: problemID,
		Status:    domain.StatusPending,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.submissions[id] = submission
	s.mu.Unlock()
	if s.repository != nil && userID != "" {
		if err := s.repository.Create(ctx, submission); err != nil {
			s.remove(id)
			return domain.Submission{}, err
		}
	}
	if err := s.enqueue(id); err != nil {
		s.remove(id)
		return domain.Submission{}, err
	}
	return submission, nil
}

// Rerun 將已完成的工作重設為 PENDING，並以相同 operatorId 再次排入佇列。
func (s *SubmissionService) Rerun(ctx context.Context, id string) (domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return domain.Submission{}, err
	}
	if err := s.ctx.Err(); err != nil {
		return domain.Submission{}, ErrServiceStopped
	}

	if _, err := s.Get(ctx, id); err != nil {
		return domain.Submission{}, err
	}
	s.mu.Lock()
	previous := s.submissions[id]
	if !previous.Status.Terminal() {
		s.mu.Unlock()
		return domain.Submission{}, ErrSubmissionNotRerunnable
	}
	queued := previous
	queued.Status = domain.StatusPending
	queued.Score = 0
	queued.StartedAt = nil
	queued.EndedAt = nil
	s.submissions[id] = queued
	s.mu.Unlock()
	if s.repository != nil && queued.UserID != "" {
		if err := s.repository.UpdateSubmission(ctx, queued); err != nil {
			s.mu.Lock()
			s.submissions[id] = previous
			s.mu.Unlock()
			return domain.Submission{}, err
		}
	}

	if err := s.enqueue(id); err != nil {
		s.mu.Lock()
		s.submissions[id] = previous
		s.mu.Unlock()
		return domain.Submission{}, err
	}
	return queued, nil
}

// Get 依提交 ID 取得目前最新的記憶體內狀態。
func (s *SubmissionService) Get(ctx context.Context, id string) (domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return domain.Submission{}, err
	}
	s.mu.RLock()
	submission, ok := s.submissions[id]
	s.mu.RUnlock()
	if !ok {
		if s.repository == nil {
			return domain.Submission{}, ErrSubmissionNotFound
		}
		persisted, err := s.repository.Get(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return domain.Submission{}, ErrSubmissionNotFound
			}
			return domain.Submission{}, err
		}
		s.mu.Lock()
		s.submissions[id] = persisted
		s.mu.Unlock()
		return persisted, nil
	}
	return submission, nil
}

// ListByUser 取得指定使用者的提交；未接 repository 時回退到記憶體資料。
func (s *SubmissionService) ListByUser(ctx context.Context, userID string) ([]domain.Submission, error) {
	if s.repository != nil {
		return s.repository.ListSubmissionsByUser(ctx, userID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var values []domain.Submission
	for _, submission := range s.submissions {
		if submission.UserID == userID || userID == "" {
			values = append(values, submission)
		}
	}
	return values, nil
}

// OpenSource 開啟指定提交最初上傳的 ZIP。
func (s *SubmissionService) OpenSource(ctx context.Context, id string) (io.ReadCloser, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.storage.OpenSource(ctx, id)
}

// OpenLog 驗證提交 ID 後開啟指定判題階段的日誌。
func (s *SubmissionService) OpenLog(ctx context.Context, id, stage string) (io.ReadCloser, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.storage.OpenLog(ctx, id, stage)
}

// Close 停止排程、取消執行中的工作，並等待所有 goroutine 結束。
func (s *SubmissionService) Close() {
	s.cancel()
	s.queueMu.Lock()
	s.stopped = true
	s.queue = nil
	s.queueCV.Broadcast()
	s.queueMu.Unlock()
	s.dispatcherWG.Wait()
	s.jobsWG.Wait()
}

// remove 從記憶體索引移除指定提交。
func (s *SubmissionService) remove(id string) {
	s.mu.Lock()
	delete(s.submissions, id)
	s.mu.Unlock()
}

// enqueue 將工作加入不受固定 channel 容量限制的 FIFO 佇列。
func (s *SubmissionService) enqueue(id string) error {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.stopped || s.ctx.Err() != nil {
		return ErrServiceStopped
	}
	s.queue = append(s.queue, id)
	s.queueCV.Signal()
	return nil
}

// dequeue 等待並取得下一個工作；服務停止時回傳 false。
func (s *SubmissionService) dequeue() (string, bool) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	for len(s.queue) == 0 && !s.stopped {
		s.queueCV.Wait()
	}
	if s.stopped {
		return "", false
	}
	id := s.queue[0]
	s.queue[0] = ""
	s.queue = s.queue[1:]
	return id, true
}

// dispatch 依 FIFO 順序取出工作，並由 semaphore 控制 JudgeRunner 併發數。
func (s *SubmissionService) dispatch() {
	defer s.dispatcherWG.Done()
	for {
		id, ok := s.dequeue()
		if !ok {
			return
		}
		select {
		case s.semaphore <- struct{}{}:
		case <-s.ctx.Done():
			return
		}

		s.jobsWG.Add(1)
		go func(jobID string) {
			defer s.jobsWG.Done()
			defer func() { <-s.semaphore }()
			s.judge(jobID)
		}(id)
	}
}

// judge 更新提交狀態、呼叫判題器並保存最終結果。
func (s *SubmissionService) judge(id string) {
	s.mu.Lock()
	submission, ok := s.submissions[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	submission.Status = domain.StatusRunning
	submission.StartedAt = &now
	s.submissions[id] = submission
	s.mu.Unlock()
	if s.repository != nil && submission.UserID != "" {
		_ = s.repository.UpdateSubmission(context.WithoutCancel(s.ctx), submission)
	}

	status, score, _ := s.runner.Run(s.ctx, submission)
	if !status.Terminal() {
		status = domain.StatusSE
		score = 0
	}
	ended := time.Now().UTC()
	s.mu.Lock()
	submission.Status = status
	submission.Score = score
	submission.EndedAt = &ended
	s.submissions[id] = submission
	s.mu.Unlock()
	if s.repository != nil && submission.UserID != "" {
		_ = s.repository.UpdateSubmission(context.WithoutCancel(s.ctx), submission)
	}
}

// newID 產生符合 UUID v4 格式的隨機提交 ID。
func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate submission id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

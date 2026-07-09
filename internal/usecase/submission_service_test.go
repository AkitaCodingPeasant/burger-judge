package usecase

// 此檔案測試非同步狀態更新與服務關閉行為。

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"regs/internal/domain"
)

type serviceTestStorage struct {
	saves atomic.Int32
}

// SaveSource 模擬儲存原始碼並記錄呼叫次數。
func (s *serviceTestStorage) SaveSource(context.Context, string, io.Reader) error {
	s.saves.Add(1)
	return nil
}

// OpenSource 回傳空的模擬原始碼內容。
func (*serviceTestStorage) OpenSource(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// SaveLog 模擬成功儲存判題日誌。
func (*serviceTestStorage) SaveLog(context.Context, string, string, io.Reader) error { return nil }

// OpenLog 回傳空的模擬判題日誌。
func (*serviceTestStorage) OpenLog(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

type serviceTestRunner struct {
	status domain.SubmissionStatus
}

// Run 回傳測試指定的判題狀態。
func (r serviceTestRunner) Run(context.Context, domain.Submission) (domain.SubmissionStatus, int, error) {
	return r.status, 25, nil
}

type blockingServiceRunner struct {
	started chan string
	release chan struct{}
	current atomic.Int32
	maximum atomic.Int32
}

func newBlockingServiceRunner() *blockingServiceRunner {
	return &blockingServiceRunner{
		started: make(chan string, 16),
		release: make(chan struct{}, 16),
	}
}

// Run 阻塞工作，讓測試能觀察 PENDING 狀態與同時執行數量。
func (r *blockingServiceRunner) Run(ctx context.Context, submission domain.Submission) (domain.SubmissionStatus, int, error) {
	current := r.current.Add(1)
	defer r.current.Add(-1)
	for {
		maximum := r.maximum.Load()
		if current <= maximum || r.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	r.started <- submission.ID
	select {
	case <-r.release:
		return domain.StatusAC, 100, nil
	case <-ctx.Done():
		return domain.StatusSE, 0, ctx.Err()
	}
}

type countingServiceRunner struct {
	calls atomic.Int32
}

func (r *countingServiceRunner) Run(context.Context, domain.Submission) (domain.SubmissionStatus, int, error) {
	r.calls.Add(1)
	return domain.StatusAC, 100, nil
}

// TestSubmissionServiceNormalizesUnknownRunnerStatus 驗證未知結果會轉換成系統錯誤。
func TestSubmissionServiceNormalizesUnknownRunnerStatus(t *testing.T) {
	service := NewSubmissionService(&serviceTestStorage{}, serviceTestRunner{status: "UNKNOWN"}, 1)
	defer service.Close()

	submission, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, err := service.Get(context.Background(), submission.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status.Terminal() {
			if got.Status != domain.StatusSE {
				t.Fatalf("status = %s, want SE", got.Status)
			}
			if got.Score != 0 {
				t.Fatalf("score = %d, want 0 for an invalid runner result", got.Score)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("submission did not reach a terminal state")
}

// TestSubmissionServiceRejectsWorkAfterClose 驗證服務關閉後不再接受提交。
func TestSubmissionServiceRejectsWorkAfterClose(t *testing.T) {
	storage := &serviceTestStorage{}
	service := NewSubmissionService(storage, serviceTestRunner{status: domain.StatusAC}, 1)
	service.Close()

	_, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source"))
	if err != ErrServiceStopped {
		t.Fatalf("Submit() error = %v, want ErrServiceStopped", err)
	}
	if storage.saves.Load() != 0 {
		t.Fatal("a stopped service must not persist a new upload")
	}
}

// TestSubmissionServiceSemaphoreKeepsOverflowPending 驗證最多只會同時執行指定數量的工作。
func TestSubmissionServiceSemaphoreKeepsOverflowPending(t *testing.T) {
	runner := newBlockingServiceRunner()
	service := NewSubmissionService(&serviceTestStorage{}, runner, 2)
	defer service.Close()

	submissions := make([]domain.Submission, 0, 3)
	for range 3 {
		started := time.Now()
		submission, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source"))
		if err != nil {
			t.Fatal(err)
		}
		if time.Since(started) > 250*time.Millisecond {
			t.Fatal("Submit blocked while all judge slots were occupied")
		}
		submissions = append(submissions, submission)
	}

	running := make(map[string]bool)
	for range 2 {
		select {
		case id := <-runner.started:
			running[id] = true
		case <-time.After(time.Second):
			t.Fatal("two jobs did not start")
		}
	}

	var pendingID string
	for _, submission := range submissions {
		if !running[submission.ID] {
			pendingID = submission.ID
			break
		}
	}
	if pendingID == "" {
		t.Fatal("could not identify queued submission")
	}
	pending, err := service.Get(context.Background(), pendingID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != domain.StatusPending {
		t.Fatalf("overflow status = %s, want PENDING", pending.Status)
	}
	if runner.maximum.Load() != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", runner.maximum.Load())
	}

	runner.release <- struct{}{}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("queued job did not start after a slot became available")
	}
	runner.release <- struct{}{}
	runner.release <- struct{}{}
	for _, submission := range submissions {
		waitForTerminalSubmission(t, service, submission.ID)
	}
}

// TestSubmissionServiceQueueDoesNotBlockWhenSlotIsBusy 驗證大量工作可立即排隊，不受固定 channel 容量限制。
func TestSubmissionServiceQueueDoesNotBlockWhenSlotIsBusy(t *testing.T) {
	runner := newBlockingServiceRunner()
	service := NewSubmissionService(&serviceTestStorage{}, runner, 1)
	defer service.Close()

	if _, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("first job did not occupy the semaphore")
	}

	started := time.Now()
	for range 100 {
		if _, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source")); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(started) > time.Second {
		t.Fatal("submitting queued jobs blocked on judge capacity")
	}
}

// TestSubmissionServiceRerunsTerminalJob 驗證完成的工作可沿用 operatorId 重跑。
func TestSubmissionServiceRerunsTerminalJob(t *testing.T) {
	runner := &countingServiceRunner{}
	service := NewSubmissionService(&serviceTestStorage{}, runner, 1)
	defer service.Close()

	submission, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	waitForTerminalSubmission(t, service, submission.ID)

	rerun, err := service.Rerun(context.Background(), submission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rerun.ID != submission.ID || rerun.Status != domain.StatusPending || rerun.Score != 0 {
		t.Fatalf("Rerun() = %+v, want same ID in PENDING with zero score", rerun)
	}
	waitForTerminalSubmission(t, service, submission.ID)
	if runner.calls.Load() != 2 {
		t.Fatalf("runner calls = %d, want 2", runner.calls.Load())
	}
}

// TestSubmissionServiceRejectsRerunWhileRunning 驗證同一工作不會被重複執行。
func TestSubmissionServiceRejectsRerunWhileRunning(t *testing.T) {
	runner := newBlockingServiceRunner()
	service := NewSubmissionService(&serviceTestStorage{}, runner, 1)
	defer service.Close()

	submission, err := service.Submit(context.Background(), "problem-1", strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	if _, err := service.Rerun(context.Background(), submission.ID); err != ErrSubmissionNotRerunnable {
		t.Fatalf("Rerun() error = %v, want ErrSubmissionNotRerunnable", err)
	}
	runner.release <- struct{}{}
}

func waitForTerminalSubmission(t *testing.T, service *SubmissionService, id string) domain.Submission {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		submission, err := service.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if submission.Status.Terminal() {
			return submission
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("submission did not reach a terminal state")
	return domain.Submission{}
}

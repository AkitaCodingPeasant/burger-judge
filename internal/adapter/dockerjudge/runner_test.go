package dockerjudge

// 此檔案使用假執行器測試判題分類，不會呼叫真正的 Docker daemon。

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"regs/internal/domain"
)

type fakeStorage struct {
	source string
	logs   map[string]string
}

// SourceDir 回傳測試指定的模擬原始碼目錄。
func (s *fakeStorage) SourceDir(string) (string, error) { return s.source, nil }

// SaveLog 將各階段日誌保存到測試用 map。
func (s *fakeStorage) SaveLog(_ context.Context, _, stage string, r io.Reader) error {
	content, _ := io.ReadAll(r)
	s.logs[stage] = string(content)
	return nil
}

type execution struct {
	output      string
	err         error
	waitContext bool
}

type fakeExecutor struct {
	results []execution
	calls   [][]string
}

// Run 依序回傳預先設定的指令執行結果。
func (e *fakeExecutor) Run(ctx context.Context, args []string, output io.Writer) error {
	e.calls = append(e.calls, slices.Clone(args))
	result := e.results[len(e.calls)-1]
	if result.waitContext {
		<-ctx.Done()
		return ctx.Err()
	}
	_, _ = io.Copy(output, bytes.NewBufferString(result.output))
	return result.err
}

// TestRunnerClassifiesExecutionTimeout 驗證執行逾時會判定為 TLE。
func TestRunnerClassifiesExecutionTimeout(t *testing.T) {
	runner, _ := testRunner(t, []execution{{}, {}, {waitContext: true}})
	runner.runTimeout = time.Millisecond
	status, _, err := runner.Run(context.Background(), domain.Submission{ID: "job-1", ProblemID: "problem-1"})
	if err != nil || status != domain.StatusTLE {
		t.Fatalf("Run() = (%s, %v), want (TLE, nil)", status, err)
	}
}

// TestRunnerPropagatesParentCancellation 驗證上層取消訊號會正確向外傳遞。
func TestRunnerPropagatesParentCancellation(t *testing.T) {
	runner, _ := testRunner(t, []execution{{waitContext: true}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, _, err := runner.Run(ctx, domain.Submission{ID: "job-1", ProblemID: "problem-1"})
	if !errors.Is(err, context.Canceled) || status != domain.StatusSE {
		t.Fatalf("Run() = (%s, %v), want (SE, context.Canceled)", status, err)
	}
}

// TestRunnerUsesDisconnectedNetworkForExecution 驗證執行階段使用無網路容器。
func TestRunnerUsesDisconnectedNetworkForExecution(t *testing.T) {
	runner, executor := testRunner(t, []execution{{output: "configured"}, {output: "built"}, {output: "passed"}})
	status, score, err := runner.Run(context.Background(), domain.Submission{ID: "job-1", ProblemID: "problem-1"})
	if err != nil || status != domain.StatusAC || score != 100 {
		t.Fatalf("Run() = (%s, %d, %v), want (AC, 100, nil)", status, score, err)
	}
	if !hasArgs(executor.calls[0], "--network", "bridge") || !hasArgs(executor.calls[1], "--network", "bridge") {
		t.Fatal("configure and compile containers must use the build network")
	}
	if !hasArgs(executor.calls[2], "--network", "none") {
		t.Fatal("execution container must use --network none")
	}
}

// TestRunnerClassifiesStageFailures 驗證各階段錯誤會映射成正確狀態。
func TestRunnerClassifiesStageFailures(t *testing.T) {
	tests := []struct {
		name    string
		results []execution
		want    domain.SubmissionStatus
	}{
		{"setup", []execution{{output: "bad cmake", err: errors.New("exit 1")}}, domain.StatusSE},
		{"compile", []execution{{}, {output: "compiler error", err: errors.New("exit 1")}}, domain.StatusCE},
		{"wrong answer", []execution{{}, {}, {output: "tests failed", err: errors.New("exit 8")}}, domain.StatusWA},
		{"runtime error", []execution{{}, {}, {output: "Segmentation fault", err: errors.New("exit 8")}}, domain.StatusRE},
	}
	for _, tt := range tests {
		// 每個子測試驗證一種判題失敗情境。
		t.Run(tt.name, func(t *testing.T) {
			runner, _ := testRunner(t, tt.results)
			status, _, _ := runner.Run(context.Background(), domain.Submission{ID: "job-1", ProblemID: "problem-1"})
			if status != tt.want {
				t.Fatalf("Run() status = %s, want %s", status, tt.want)
			}
		})
	}
}

// testRunner 建立含假儲存與假指令執行器的判題器。
func testRunner(t *testing.T, results []execution) (*Runner, *fakeExecutor) {
	t.Helper()
	root := t.TempDir()
	problem := filepath.Join(root, "problems", "problem-1")
	if err := os.MkdirAll(problem, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(problem, "CMakeLists.txt"), []byte("project(example)"), 0o640); err != nil {
		t.Fatal(err)
	}
	storage := &fakeStorage{source: filepath.Join(root, "source"), logs: make(map[string]string)}
	executor := &fakeExecutor{results: results}
	runner := New("judge-image", filepath.Join(root, "problems"), filepath.Join(root, "builds"), time.Second, time.Second, storage)
	runner.executor = executor
	return runner, executor
}

// hasArgs 檢查相鄰的兩個命令列參數是否存在。
func hasArgs(args []string, first, second string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == first && args[i+1] == second {
			return true
		}
	}
	return false
}

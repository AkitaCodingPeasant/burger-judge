package dockerjudge

// 此檔案將各判題階段轉換成具資源限制的 Docker CLI 指令。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"regs/internal/domain"
)

type logStorage interface {
	SaveLog(context.Context, string, string, io.Reader) error
	SourceDir(string) (string, error)
}

type commandExecutor interface {
	Run(context.Context, []string, io.Writer) error
}

type dockerCLI struct{}

// Run 呼叫 Docker CLI 並將標準輸出與錯誤寫入同一份日誌。
func (dockerCLI) Run(ctx context.Context, args []string, output io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// Runner 在容器中設定、編譯並執行單次提交。
type Runner struct {
	image        string
	problemsRoot string
	buildRoot    string
	buildTimeout time.Duration
	runTimeout   time.Duration
	storage      logStorage
	executor     commandExecutor
}

// New 建立以 Docker 為執行環境的判題器。
func New(image, problemsRoot, buildRoot string, buildTimeout, runTimeout time.Duration, storage logStorage) *Runner {
	return &Runner{
		image:        image,
		problemsRoot: problemsRoot,
		buildRoot:    buildRoot,
		buildTimeout: buildTimeout,
		runTimeout:   runTimeout,
		storage:      storage,
		executor:     dockerCLI{},
	}
}

// Run 執行三個判題階段，並將結果轉換成提交狀態與分數。
func (r *Runner) Run(ctx context.Context, submission domain.Submission) (domain.SubmissionStatus, int, error) {
	sourceDir, err := r.storage.SourceDir(submission.ID)
	if err != nil {
		return domain.StatusSE, 0, err
	}
	problemDir, err := safeChild(r.problemsRoot, submission.ProblemID)
	if err != nil {
		return domain.StatusSE, 0, err
	}
	if _, err := os.Stat(filepath.Join(problemDir, "CMakeLists.txt")); err != nil {
		return domain.StatusSE, 0, fmt.Errorf("problem has no CMakeLists.txt: %w", err)
	}
	buildDir, err := safeChild(r.buildRoot, submission.ID)
	if err != nil {
		return domain.StatusSE, 0, err
	}
	if err := os.RemoveAll(buildDir); err != nil {
		return domain.StatusSE, 0, err
	}
	if err := os.MkdirAll(buildDir, 0o750); err != nil {
		return domain.StatusSE, 0, err
	}

	base := r.containerArgs("bridge",
		mount(sourceDir, "/judge/source", true),
		mount(problemDir, "/judge/problem", true),
		mount(buildDir, "/judge/build", false),
	)
	configure := append(append([]string{}, base...), r.image,
		"cmake", "-S", "/judge/problem", "-B", "/judge/build", "-G", "Ninja", "-D", "SOURCE_ROOT=/judge/source")
	if failed, err := r.stage(ctx, submission.ID, "configure", configure); failed {
		return domain.StatusSE, 0, err
	}

	compile := append(append([]string{}, base...), r.image,
		"cmake", "--build", "/judge/build", "--parallel", "--verbose")
	if failed, err := r.stage(ctx, submission.ID, "compile", compile); failed {
		return domain.StatusCE, 0, err
	}

	runArgs := r.containerArgs("none", mount(buildDir, "/judge/build", false))
	run := append(append([]string{}, runArgs...), r.image,
		"ctest", "--test-dir", "/judge/build", "--output-on-failure")
	output, runErr, timedOut := r.execute(ctx, run, r.runTimeout)
	if err := r.storage.SaveLog(context.WithoutCancel(ctx), submission.ID, "run", bytes.NewReader(output)); err != nil {
		return domain.StatusRE, 0, err
	}
	if err := ctx.Err(); err != nil {
		return domain.StatusSE, 0, err
	}
	if timedOut {
		return domain.StatusTLE, 0, nil
	}
	if runErr == nil {
		return domain.StatusAC, 100, nil
	}
	if runtimeFailure(output) {
		return domain.StatusRE, 0, nil
	}
	return domain.StatusWA, 0, nil
}

// stage 執行設定或編譯階段，並保存該階段的輸出日誌。
func (r *Runner) stage(ctx context.Context, id, name string, args []string) (bool, error) {
	output, runErr, _ := r.execute(ctx, args, r.buildTimeout)
	if err := r.storage.SaveLog(context.WithoutCancel(ctx), id, name, bytes.NewReader(output)); err != nil {
		return true, err
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	return runErr != nil, runErr
}

// execute 在指定時間限制內執行指令，並回傳輸出、錯誤及逾時狀態。
func (r *Runner) execute(parent context.Context, args []string, timeout time.Duration) ([]byte, error, bool) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var output bytes.Buffer
	err := r.executor.Run(ctx, args, &output)
	return output.Bytes(), err, errors.Is(ctx.Err(), context.DeadlineExceeded)
}

// containerArgs 建立包含網路、資源與掛載限制的 Docker 參數。
func (r *Runner) containerArgs(network string, mounts ...string) []string {
	args := []string{
		"run", "--rm", "--network", network,
		"--cpus", "1", "--memory", "512m", "--pids-limit", "128",
		"--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
	}
	for _, value := range mounts {
		args = append(args, "--mount", value)
	}
	return args
}

// mount 建立 Docker bind mount 的參數字串。
func mount(source, target string, readOnly bool) string {
	value := "type=bind,src=" + filepath.Clean(source) + ",dst=" + target
	if readOnly {
		value += ",readonly"
	}
	return value
}

// safeChild 驗證單一路徑元件並回傳根目錄下的絕對路徑。
func safeChild(root, name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid path component %q", name)
	}
	return filepath.Abs(filepath.Join(root, name))
}

// runtimeFailure 從測試輸出判斷是否發生常見的執行期錯誤。
func runtimeFailure(output []byte) bool {
	text := strings.ToLower(string(output))
	for _, marker := range []string{
		"segmentation fault", "segfault", "subprocess aborted", "illegal instruction",
		"exception:", "child aborted", "access violation",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

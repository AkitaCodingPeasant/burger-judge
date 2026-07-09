package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"regs/internal/adapter/dockerjudge"
	"regs/internal/adapter/filestorage"
	"regs/internal/adapter/httpapi"
	"regs/internal/domain"
	"regs/internal/usecase"
)

// TestBasic20Pipeline covers the complete minimum-rubric path: multipart ZIP
// upload, asynchronous judging, Docker/CMake/CTest, result polling, and all
// three stage-log endpoints.
func TestBasic20Pipeline(t *testing.T) {
	if os.Getenv("REGS_DOCKER_INTEGRATION") != "1" {
		t.Skip("set REGS_DOCKER_INTEGRATION=1 to run Docker integration tests")
	}

	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	problemsRoot := filepath.Join(repositoryRoot, "test-framework")
	storage := filestorage.New(filepath.Join(t.TempDir(), "uploads"))
	runner := dockerjudge.New(
		"yhlib/cs3060701",
		problemsRoot,
		filepath.Join(t.TempDir(), "builds"),
		2*time.Minute,
		10*time.Second,
		storage,
	)
	service := usecase.NewSubmissionService(storage, runner, 1)
	defer service.Close()
	handler := httpapi.NewFullRouter(httpapi.FullDependencies{
		Auth:        integrationAuth{},
		Submissions: service,
	})

	archive := zipDirectory(t, filepath.Join(problemsRoot, "113final003", "solution"))
	var requestBody bytes.Buffer
	form := multipart.NewWriter(&requestBody)
	if err := form.WriteField("problemId", "113final003"); err != nil {
		t.Fatal(err)
	}
	file, err := form.CreateFormFile("source", "113final003-ac.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	request := integrationRequest(http.MethodPost, "/api/submissions", &requestBody)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST /api/submissions = %d: %s", response.Code, response.Body.String())
	}
	var accepted struct {
		OperatorID string `json:"operatorId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.OperatorID == "" {
		t.Fatal("submission response has no operatorId")
	}

	deadline := time.Now().Add(2 * time.Minute)
	var result struct {
		Status domain.SubmissionStatus `json:"status"`
		Score  int                     `json:"score"`
	}
	for {
		response = httptest.NewRecorder()
		request = integrationRequest(http.MethodGet, "/api/submissions/"+accepted.OperatorID, nil)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET submission = %d: %s", response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status.Terminal() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("submission did not finish before deadline")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if result.Status != domain.StatusAC || result.Score != 100 {
		t.Fatalf("submission result = (%s, %d), want (AC, 100)", result.Status, result.Score)
	}

	for _, stage := range []string{"configure", "compile", "run"} {
		response = httptest.NewRecorder()
		request = integrationRequest(http.MethodGet, "/api/submissions/"+accepted.OperatorID+"/logs/"+stage, nil)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s log = (%d, %d bytes)", stage, response.Code, response.Body.Len())
		}
	}

	response = httptest.NewRecorder()
	request = integrationRequest(http.MethodPost, "/api/submissions/"+accepted.OperatorID+"/rerun", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST rerun = %d: %s", response.Code, response.Body.String())
	}
	var requeued struct {
		OperatorID string                  `json:"operatorId"`
		Status     domain.SubmissionStatus `json:"status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &requeued); err != nil {
		t.Fatal(err)
	}
	if requeued.OperatorID != accepted.OperatorID || requeued.Status != domain.StatusPending {
		t.Fatalf("rerun response = %+v, want same operatorId in PENDING", requeued)
	}

	deadline = time.Now().Add(2 * time.Minute)
	for {
		response = httptest.NewRecorder()
		request = integrationRequest(http.MethodGet, "/api/submissions/"+accepted.OperatorID, nil)
		handler.ServeHTTP(response, request)
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status.Terminal() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rerun did not finish before deadline")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if result.Status != domain.StatusAC || result.Score != 100 {
		t.Fatalf("rerun result = (%s, %d), want (AC, 100)", result.Status, result.Score)
	}
}

type integrationAuth struct{}

func (integrationAuth) Register(context.Context, string, string) (domain.User, error) {
	return domain.User{}, nil
}

func (integrationAuth) Login(context.Context, string, string) (string, domain.User, error) {
	return "", domain.User{}, nil
}

func (integrationAuth) Authenticate(context.Context, string) (domain.Principal, error) {
	return domain.Principal{UserID: "integration-user", Role: domain.RoleUser}, nil
}

func (integrationAuth) Logout(context.Context, domain.Principal) error { return nil }

func (integrationAuth) Me(context.Context, string) (domain.User, error) {
	return domain.User{}, nil
}

func integrationRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Authorization", "Bearer integration-token")
	return request
}

func zipDirectory(t *testing.T, root string) []byte {
	t.Helper()
	var output bytes.Buffer
	w := zip.NewWriter(&output)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		destination, err := w.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

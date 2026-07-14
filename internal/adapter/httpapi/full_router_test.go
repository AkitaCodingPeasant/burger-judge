package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"regs/internal/domain"
	"regs/internal/usecase"
)

type fakeAuthentication struct {
	principals map[string]domain.Principal
}

// TestFullRouterServesSwaggerUI 驗證 Swagger UI 與規格皆由 API 同源提供。
func TestFullRouterServesSwaggerUI(t *testing.T) {
	specPath := t.TempDir() + "/openapi.yaml"
	if err := os.WriteFile(specPath, []byte("openapi: 3.0.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := NewFullRouter(FullDependencies{OpenAPISpec: specPath})

	for _, test := range []struct {
		path        string
		contentType string
		body        string
	}{
		{path: "/docs", contentType: "text/html", body: "/openapi.yaml"},
		{path: "/openapi.yaml", contentType: "application/yaml", body: "openapi: 3.0.3"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", test.path, response.Code)
		}
		if !strings.Contains(response.Header().Get("Content-Type"), test.contentType) {
			t.Fatalf("GET %s Content-Type = %q", test.path, response.Header().Get("Content-Type"))
		}
		if !strings.Contains(response.Body.String(), test.body) {
			t.Fatalf("GET %s body does not contain %q", test.path, test.body)
		}
	}
}

func (f *fakeAuthentication) Register(context.Context, string, string) (domain.User, error) {
	return domain.User{}, errors.New("not used")
}
func (f *fakeAuthentication) Login(context.Context, string, string) (string, domain.User, error) {
	return "", domain.User{}, errors.New("not used")
}
func (f *fakeAuthentication) Authenticate(_ context.Context, token string) (domain.Principal, error) {
	principal, ok := f.principals[token]
	if !ok {
		return domain.Principal{}, errors.New("invalid token")
	}
	return principal, nil
}
func (f *fakeAuthentication) Logout(context.Context, domain.Principal) error { return nil }
func (f *fakeAuthentication) Me(_ context.Context, id string) (domain.User, error) {
	return domain.User{ID: id, Account: "alice", Role: domain.RoleUser}, nil
}

func TestFullRouterRequiresBearerToken(t *testing.T) {
	router := NewFullRouter(FullDependencies{Auth: &fakeAuthentication{principals: map[string]domain.Principal{}}})
	request := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestFullRouterAllowsUserAndAdminOnUserEndpoint(t *testing.T) {
	auth := &fakeAuthentication{principals: map[string]domain.Principal{
		"user-token":  {UserID: "user-1", Role: domain.RoleUser},
		"admin-token": {UserID: "admin-1", Role: domain.RoleAdmin},
	}}
	router := NewFullRouter(FullDependencies{Auth: auth})
	for _, token := range []string{"user-token", "admin-token"} {
		request := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("token %s status = %d, want 200", token, response.Code)
		}
	}
}

func TestFullRouterRejectsUserOnAdminEndpoint(t *testing.T) {
	auth := &fakeAuthentication{principals: map[string]domain.Principal{
		"user-token": {UserID: "user-1", Role: domain.RoleUser},
	}}
	router := NewFullRouter(FullDependencies{Auth: auth})
	request := httptest.NewRequest(http.MethodPut, "/api/problems", nil)
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

type fakeFullSubmissionService struct {
	submission domain.Submission
	log        string
	submitErr  error
	rerunErr   error
}

func (s *fakeFullSubmissionService) SubmitForUser(_ context.Context, userID, problemID string, _ io.Reader) (domain.Submission, error) {
	if s.submitErr != nil {
		return domain.Submission{}, s.submitErr
	}
	s.submission = domain.Submission{ID: "operator-1", UserID: userID, ProblemID: problemID, Status: domain.StatusPending, CreatedAt: time.Now()}
	return s.submission, nil
}

func (s *fakeFullSubmissionService) Rerun(_ context.Context, id string) (domain.Submission, error) {
	if s.rerunErr != nil {
		return domain.Submission{}, s.rerunErr
	}
	s.submission.ID = id
	s.submission.Status = domain.StatusPending
	s.submission.Score = 0
	return s.submission, nil
}

func (s *fakeFullSubmissionService) Get(context.Context, string) (domain.Submission, error) {
	return s.submission, nil
}

func (s *fakeFullSubmissionService) ListByUser(context.Context, string) ([]domain.Submission, error) {
	return []domain.Submission{s.submission}, nil
}

func (s *fakeFullSubmissionService) OpenSource(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("source")), nil
}

func (s *fakeFullSubmissionService) OpenLog(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.log)), nil
}

func routerWithUser(service fullSubmissionService) http.Handler {
	auth := &fakeAuthentication{principals: map[string]domain.Principal{
		"user-token": {UserID: "user-1", Role: domain.RoleUser},
	}}
	return NewFullRouter(FullDependencies{Auth: auth, Submissions: service})
}

func submissionRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("problemId", "113final001"); err != nil {
		t.Fatal(err)
	}
	file, err := form.CreateFormFile("source", "source.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("zip contents")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/submissions", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer user-token")
	return request
}

func TestFullRouterSubmissionReturnsOperatorID(t *testing.T) {
	response := httptest.NewRecorder()
	routerWithUser(&fakeFullSubmissionService{}).ServeHTTP(response, submissionRequest(t))
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"operatorId":"operator-1"`) {
		t.Fatalf("response = (%d, %s)", response.Code, response.Body.String())
	}
}

func TestFullRouterSubmissionRejectsInvalidSource(t *testing.T) {
	response := httptest.NewRecorder()
	routerWithUser(&fakeFullSubmissionService{submitErr: usecase.ErrInvalidSource}).ServeHTTP(response, submissionRequest(t))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestFullRouterRerunAndLog(t *testing.T) {
	service := &fakeFullSubmissionService{
		submission: domain.Submission{ID: "operator-1", UserID: "user-1", Status: domain.StatusAC, Score: 100},
		log:        "compiler output",
	}
	router := routerWithUser(service)

	request := httptest.NewRequest(http.MethodPost, "/api/submissions/operator-1/rerun", nil)
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("rerun response = (%d, %s)", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/submissions/operator-1/logs/compile", nil)
	request.Header.Set("Authorization", "Bearer user-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != service.log {
		t.Fatalf("log response = (%d, %q)", response.Code, response.Body.String())
	}
}

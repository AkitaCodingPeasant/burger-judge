package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"regs/internal/domain"
	"regs/internal/usecase"
)

type fullSubmissionService interface {
	SubmitForUser(context.Context, string, string, io.Reader) (domain.Submission, error)
	Rerun(context.Context, string) (domain.Submission, error)
	Get(context.Context, string) (domain.Submission, error)
	ListByUser(context.Context, string) ([]domain.Submission, error)
	OpenSource(context.Context, string) (io.ReadCloser, error)
	OpenLog(context.Context, string, string) (io.ReadCloser, error)
}

type authenticationService interface {
	Register(context.Context, string, string) (domain.User, error)
	Login(context.Context, string, string) (string, domain.User, error)
	Authenticate(context.Context, string) (domain.Principal, error)
	Logout(context.Context, domain.Principal) error
	Me(context.Context, string) (domain.User, error)
}

type problemService interface {
	List(context.Context) ([]domain.Problem, error)
	Get(context.Context, string) (domain.Problem, error)
	Upsert(context.Context, string, string, string, io.Reader) (domain.Problem, error)
	Delete(context.Context, string) error
	OpenArchive(context.Context, string) (io.ReadCloser, error)
}

type statisticsService interface {
	ProblemStatistics(context.Context, string) (domain.ProblemStatistics, error)
	UserStatistics(context.Context, string) (domain.UserStatistics, error)
}

type FullDependencies struct {
	Auth        authenticationService
	Problems    problemService
	Submissions fullSubmissionService
	Statistics  statisticsService
	OpenAPISpec string
}

type principalContextKey struct{}

// NewFullRouter 註冊作業規格的完整 JWT/RBAC API。
func NewFullRouter(deps FullDependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs", http.StatusFound)
	})
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /docs", swaggerUI)
	mux.HandleFunc("GET /docs/", swaggerUI)
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPISpec(deps.OpenAPISpec))

	mux.HandleFunc("POST /api/users/register", register(deps.Auth))
	mux.HandleFunc("POST /api/users/login", login(deps.Auth))
	mux.HandleFunc("POST /api/users/logout", requireRole(deps.Auth, domain.RoleUser, logout(deps.Auth)))
	mux.HandleFunc("GET /api/users/me", requireRole(deps.Auth, domain.RoleUser, me(deps.Auth)))
	mux.HandleFunc("GET /api/users/{user_id}/submissions", listUserSubmissions(deps.Submissions))

	mux.HandleFunc("GET /api/problems", listProblems(deps.Problems))
	mux.HandleFunc("GET /api/problems/{problem_id}", getProblem(deps.Problems))
	mux.HandleFunc("PUT /api/problems", requireRole(deps.Auth, domain.RoleAdmin, putProblem(deps.Problems)))
	mux.HandleFunc("DELETE /api/problems/{problem_id}", requireRole(deps.Auth, domain.RoleAdmin, deleteProblem(deps.Problems)))
	mux.HandleFunc("GET /api/problems/{problem_id}/testcases", requireRole(deps.Auth, domain.RoleAdmin, downloadProblem(deps.Problems)))

	mux.HandleFunc("POST /api/submissions", requireRole(deps.Auth, domain.RoleUser, submitForUser(deps.Submissions)))
	mux.HandleFunc("GET /api/submissions", requireRole(deps.Auth, domain.RoleUser, listOwnSubmissions(deps.Submissions)))
	mux.HandleFunc("GET /api/submissions/{operatorId}", requireRole(deps.Auth, domain.RoleUser, getOwnedSubmission(deps.Submissions)))
	mux.HandleFunc("GET /api/submissions/{operatorId}/source", requireRole(deps.Auth, domain.RoleUser, getSource(deps.Submissions)))
	mux.HandleFunc("POST /api/submissions/{operatorId}/rerun", requireRole(deps.Auth, domain.RoleUser, rerunOwnedSubmission(deps.Submissions)))
	mux.HandleFunc("GET /api/submissions/{operatorId}/logs/{stage}", requireRole(deps.Auth, domain.RoleUser, getOwnedLog(deps.Submissions)))

	mux.HandleFunc("GET /api/stats/problems/{problem_id}", problemStats(deps.Statistics))
	mux.HandleFunc("GET /api/stats/users/{user_id}", userStats(deps.Statistics))
	return allowCORS(logRequests(mux))
}

// logRequests 輸出每一筆進入 API 的基本請求資訊。
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if !shouldLogRequest(r) {
			return
		}
		log.Printf("API 請求: %s || %s || %d || duration=%s || remote=%s", r.Method, r.URL.Path, recorder.status, time.Since(started).Round(time.Millisecond), r.RemoteAddr)
	})
}

// shouldLogRequest 判斷這筆請求是否需要顯示在除錯輸出。
func shouldLogRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader 記錄 handler 實際回傳的 HTTP 狀態碼。
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireRole(auth authenticationService, role domain.Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "bearer token required")
			return
		}
		principal, err := auth.Authenticate(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if !principal.Role.Allows(role) {
			writeError(w, http.StatusForbidden, "permission denied")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
	}
}

func principalFrom(ctx context.Context) domain.Principal {
	principal, _ := ctx.Value(principalContextKey{}).(domain.Principal)
	return principal
}

type credentialsRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        string      `json:"id"`
	Account   string      `json:"account"`
	Role      domain.Role `json:"role"`
	CreatedAt any         `json:"createdAt"`
}

func toUserResponse(user domain.User) userResponse {
	return userResponse{ID: user.ID, Account: user.Account, Role: user.Role, CreatedAt: user.CreatedAt}
}

func register(auth authenticationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request credentialsRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		user, err := auth.Register(r.Context(), request.Account, request.Password)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toUserResponse(user))
	}
}

func login(auth authenticationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request credentialsRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		token, user, err := auth.Login(r.Context(), request.Account, request.Password)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": toUserResponse(user)})
	}
}

func logout(auth authenticationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := auth.Logout(r.Context(), principalFrom(r.Context())); err != nil {
			writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func me(auth authenticationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.Me(r.Context(), principalFrom(r.Context()).UserID)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserResponse(user))
	}
}

func listProblems(service problemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values, err := service.List(r.Context())
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, values)
	}
}

func getProblem(service problemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := service.Get(r.Context(), r.PathValue("problem_id"))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
}

func putProblem(service problemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestSize)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart problem")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		archive, _, err := r.FormFile("archive")
		if err != nil {
			writeError(w, http.StatusBadRequest, "problem archive is required")
			return
		}
		defer archive.Close()
		value, err := service.Upsert(r.Context(), r.FormValue("problemId"), r.FormValue("title"), r.FormValue("description"), archive)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
}

func deleteProblem(service problemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := service.Delete(r.Context(), r.PathValue("problem_id")); err != nil {
			writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func downloadProblem(service problemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("problem_id")
		archive, err := service.OpenArchive(r.Context(), id)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		defer archive.Close()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.zip"`)
		_, _ = io.Copy(w, archive)
	}
}

func submitForUser(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestSize)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart submission")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, _, err := r.FormFile("source")
		if err != nil {
			writeError(w, http.StatusBadRequest, "source ZIP is required")
			return
		}
		defer file.Close()
		principal := principalFrom(r.Context())
		submission, err := service.SubmitForUser(r.Context(), principal.UserID, strings.TrimSpace(r.FormValue("problemId")), file)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"operatorId": submission.ID, "status": string(submission.Status)})
	}
}

func listOwnSubmissions(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values, err := service.ListByUser(r.Context(), principalFrom(r.Context()).UserID)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, values)
	}
}

func listUserSubmissions(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values, err := service.ListByUser(r.Context(), r.PathValue("user_id"))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, values)
	}
}

func getOwnedSubmission(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		submission, err := ownedSubmission(r.Context(), service, r.PathValue("operatorId"))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, submission)
	}
}

func getSource(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("operatorId")
		if _, err := ownedSubmission(r.Context(), service, id); err != nil {
			writeAPIError(w, err)
			return
		}
		source, err := service.OpenSource(r.Context(), id)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		defer source.Close()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.zip"`)
		_, _ = io.Copy(w, source)
	}
}

func rerunOwnedSubmission(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("operatorId")
		if _, err := ownedSubmission(r.Context(), service, id); err != nil {
			writeAPIError(w, err)
			return
		}
		submission, err := service.Rerun(r.Context(), id)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"operatorId": submission.ID, "status": string(submission.Status)})
	}
}

func getOwnedLog(service fullSubmissionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("operatorId")
		if _, err := ownedSubmission(r.Context(), service, id); err != nil {
			writeAPIError(w, err)
			return
		}
		stage := r.PathValue("stage")
		if stage != "configure" && stage != "compile" && stage != "run" {
			writeError(w, http.StatusBadRequest, "unknown log stage")
			return
		}
		log, err := service.OpenLog(r.Context(), id, stage)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		defer log.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.Copy(w, log)
	}
}

func ownedSubmission(ctx context.Context, service fullSubmissionService, id string) (domain.Submission, error) {
	submission, err := service.Get(ctx, id)
	if err != nil {
		return domain.Submission{}, err
	}
	principal := principalFrom(ctx)
	if principal.Role != domain.RoleAdmin && submission.UserID != principal.UserID {
		return domain.Submission{}, usecase.ErrForbidden
	}
	return submission, nil
}

func problemStats(service statisticsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := service.ProblemStatistics(r.Context(), r.PathValue("problem_id"))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
}

func userStats(service statisticsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := service.UserStatistics(r.Context(), r.PathValue("user_id"))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func writeAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, usecase.ErrSubmissionNotFound), errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, usecase.ErrConflict), errors.Is(err, usecase.ErrSubmissionNotRerunnable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, usecase.ErrInvalidCredentials), errors.Is(err, usecase.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, usecase.ErrForbidden):
		writeError(w, http.StatusForbidden, "permission denied")
	case errors.Is(err, usecase.ErrInvalidAccount), errors.Is(err, usecase.ErrWeakPassword), errors.Is(err, usecase.ErrInvalidProblem), errors.Is(err, usecase.ErrInvalidSource):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "request failed")
	}
}

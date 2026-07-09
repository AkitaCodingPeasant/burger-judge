package dockerjudge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"regs/internal/domain"
)

// TestRunnerWithOfficialSolutions exercises the real Docker/CMake/CTest
// pipeline. It is opt-in so the regular unit-test suite remains usable on
// machines without Docker Desktop.
func TestRunnerWithOfficialSolutions(t *testing.T) {
	if os.Getenv("REGS_DOCKER_INTEGRATION") != "1" {
		t.Skip("set REGS_DOCKER_INTEGRATION=1 to run Docker integration tests")
	}

	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	problemsRoot := filepath.Join(repositoryRoot, "test-framework")

	for _, problemID := range []string{
		"113final001",
		"113final003",
		"113final004",
		"113final005",
		"113final006",
	} {
		t.Run(problemID, func(t *testing.T) {
			storage := &fakeStorage{
				source: filepath.Join(problemsRoot, problemID, "solution"),
				logs:   make(map[string]string),
			}
			runner := New(
				"yhlib/cs3060701",
				problemsRoot,
				filepath.Join(t.TempDir(), "builds"),
				2*time.Minute,
				10*time.Second,
				storage,
			)

			status, score, err := runner.Run(context.Background(), domain.Submission{
				ID:        "verify-" + problemID,
				ProblemID: problemID,
			})
			if err != nil || status != domain.StatusAC || score != 100 {
				t.Fatalf("Run() = (%s, %d, %v), want (AC, 100, nil)", status, score, err)
			}
			for _, stage := range []string{"configure", "compile", "run"} {
				if storage.logs[stage] == "" {
					t.Errorf("%s log is empty", stage)
				}
			}
		})
	}
}

// TestRunnerClassifiesRealPrograms proves the four rubric statuses with real
// compiler and process behaviour rather than mocked command results.
func TestRunnerClassifiesRealPrograms(t *testing.T) {
	if os.Getenv("REGS_DOCKER_INTEGRATION") != "1" {
		t.Skip("set REGS_DOCKER_INTEGRATION=1 to run Docker integration tests")
	}

	root := t.TempDir()
	problemsRoot := filepath.Join(root, "problems")
	problemRoot := filepath.Join(problemsRoot, "status-demo")
	writeIntegrationFile(t, filepath.Join(problemRoot, "CMakeLists.txt"), `
cmake_minimum_required(VERSION 3.20)
project(status_demo LANGUAGES CXX)
enable_testing()
add_executable(status-demo "${CMAKE_CURRENT_SOURCE_DIR}/judge_main.cpp" "${SOURCE_ROOT}/answer.cpp")
target_include_directories(status-demo PRIVATE "${SOURCE_ROOT}")
add_test(NAME status-demo COMMAND status-demo)
`)
	writeIntegrationFile(t, filepath.Join(problemRoot, "judge_main.cpp"), `
#include "answer.h"
#include <iostream>
int main() {
  const int actual = answer();
  if (actual != 42) {
    std::cerr << "wrong answer: expected 42, got " << actual << '\n';
    return 1;
  }
  return 0;
}
`)

	tests := []struct {
		name   string
		source string
		want   domain.SubmissionStatus
	}{
		{name: "ac", source: "int answer() { return 42; }\n", want: domain.StatusAC},
		{name: "wa", source: "int answer() { return 7; }\n", want: domain.StatusWA},
		{name: "ce", source: "int answer( { return 42; }\n", want: domain.StatusCE},
		{name: "re", source: "int answer() { int *p = nullptr; return *p; }\n", want: domain.StatusRE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := filepath.Join(root, "sources", tt.name)
			writeIntegrationFile(t, filepath.Join(sourceRoot, "CMakeLists.txt"), "cmake_minimum_required(VERSION 3.20)\nproject(submission)\n")
			writeIntegrationFile(t, filepath.Join(sourceRoot, "answer.h"), "int answer();\n")
			writeIntegrationFile(t, filepath.Join(sourceRoot, "answer.cpp"), tt.source)

			storage := &fakeStorage{source: sourceRoot, logs: make(map[string]string)}
			runner := New(
				"yhlib/cs3060701",
				problemsRoot,
				filepath.Join(root, "builds", tt.name),
				2*time.Minute,
				10*time.Second,
				storage,
			)
			status, _, _ := runner.Run(context.Background(), domain.Submission{
				ID:        "status-" + tt.name,
				ProblemID: "status-demo",
			})
			if status != tt.want {
				t.Fatalf("Run() status = %s, want %s; run log: %s", status, tt.want, storage.logs["run"])
			}
		})
	}
}

func writeIntegrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

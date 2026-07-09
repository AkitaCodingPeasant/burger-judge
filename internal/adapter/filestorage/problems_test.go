package filestorage

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestProblemStoreSaveAndOpen(t *testing.T) {
	root := t.TempDir()
	store := NewProblemStore(root)
	archive := problemZIP(t, map[string]string{
		"CMakeLists.txt": "cmake_minimum_required(VERSION 3.16)",
		"spec/case.h":    "// testcase",
	})
	path, err := store.SaveProblem(context.Background(), "problem001", bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "problem001", "settings.yaml") {
		t.Fatalf("unexpected settings path %q", path)
	}
	if _, err := os.Stat(filepath.Join(root, "problem001", "CMakeLists.txt")); err != nil {
		t.Fatal(err)
	}
	file, err := store.OpenProblem(context.Background(), "problem001")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opened, archive) {
		t.Fatal("downloaded archive differs from uploaded archive")
	}
}

func TestProblemStoreRejectsMissingCMakeLists(t *testing.T) {
	store := NewProblemStore(t.TempDir())
	archive := problemZIP(t, map[string]string{"README.md": "missing project"})
	if _, err := store.SaveProblem(context.Background(), "problem001", bytes.NewReader(archive)); err == nil {
		t.Fatal("SaveProblem() accepted an archive without CMakeLists.txt")
	}
}

func problemZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(file, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

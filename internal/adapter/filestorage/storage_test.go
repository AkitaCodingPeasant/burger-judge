package filestorage

// 此檔案測試正常解壓縮、路徑安全與分階段日誌。

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestSaveSourceExtractsCMakeProject 驗證合法 CMake 專案可正確解壓縮。
func TestSaveSourceExtractsCMakeProject(t *testing.T) {
	storage := New(t.TempDir())
	archive := makeZIP(t, map[string]string{
		"CMakeLists.txt": "project(example)",
		"src/main.cpp":   "int main() {}",
	})
	if err := storage.SaveSource(context.Background(), "submission-1", archive); err != nil {
		t.Fatalf("SaveSource() error = %v", err)
	}
	dir, err := storage.SourceDir("submission-1")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "src", "main.cpp"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "int main() {}" {
		t.Fatalf("unexpected source: %q", content)
	}
}

// TestSaveSourceRejectsZIPSlip 驗證解壓縮時會拒絕路徑穿越。
func TestSaveSourceRejectsZIPSlip(t *testing.T) {
	storage := New(t.TempDir())
	err := storage.SaveSource(context.Background(), "submission-1", makeZIP(t, map[string]string{
		"../outside":     "bad",
		"CMakeLists.txt": "project(example)",
	}))
	if !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("SaveSource() error = %v, want ErrInvalidArchive", err)
	}
}

// TestSaveSourceRequiresCMakeListsAtRoot 驗證專案根目錄必須包含 CMakeLists.txt。
func TestSaveSourceRequiresCMakeListsAtRoot(t *testing.T) {
	storage := New(t.TempDir())
	err := storage.SaveSource(context.Background(), "submission-1", makeZIP(t, map[string]string{
		"main.cpp": "int main() {}",
	}))
	if !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("SaveSource() error = %v, want ErrInvalidArchive", err)
	}
}

// TestSaveSourceRejectsCMakeListsDirectory 驗證同名目錄不能冒充 CMakeLists.txt。
func TestSaveSourceRejectsCMakeListsDirectory(t *testing.T) {
	storage := New(t.TempDir())
	err := storage.SaveSource(context.Background(), "submission-1", makeZIP(t, map[string]string{
		"CMakeLists.txt/": "",
	}))
	if !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("SaveSource() error = %v, want ErrInvalidArchive", err)
	}
	if _, err := storage.OpenSource(context.Background(), "submission-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid archive was not removed: %v", err)
	}
}

// TestStageLogsAreSeparate 驗證不同判題階段的日誌會分開保存。
func TestStageLogsAreSeparate(t *testing.T) {
	storage := New(t.TempDir())
	for stage, content := range map[string]string{"configure": "cfg", "compile": "cc", "run": "out"} {
		if err := storage.SaveLog(context.Background(), "submission-1", stage, bytes.NewBufferString(content)); err != nil {
			t.Fatal(err)
		}
		log, err := storage.OpenLog(context.Background(), "submission-1", stage)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(log)
		log.Close()
		if string(got) != content {
			t.Fatalf("%s log = %q, want %q", stage, got, content)
		}
	}
}

// makeZIP 依指定檔案內容建立測試用 ZIP。
func makeZIP(t *testing.T, files map[string]string) io.Reader {
	t.Helper()
	var output bytes.Buffer
	w := zip.NewWriter(&output)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(output.Bytes())
}

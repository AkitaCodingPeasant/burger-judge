package filestorage

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ProblemStore 保存管理者上傳的完整題目目錄與原始 ZIP。
type ProblemStore struct {
	root string
}

func NewProblemStore(root string) *ProblemStore { return &ProblemStore{root: root} }

func (s *ProblemStore) SaveProblem(ctx context.Context, id string, source io.Reader) (string, error) {
	if err := validateName(id); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(s.root, ".problem-"+id+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)

	archivePath := filepath.Join(staging, "problem.zip")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(archive, io.LimitReader(source, maxArchiveSize+1))
	closeErr := archive.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > maxArchiveSize {
		return "", fmt.Errorf("%w: problem archive exceeds %d bytes", ErrInvalidArchive, maxArchiveSize)
	}

	content := filepath.Join(staging, "content")
	if err := extractZIP(ctx, archivePath, content); err != nil {
		return "", err
	}
	info, err := os.Stat(filepath.Join(content, "CMakeLists.txt"))
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: problem root must contain CMakeLists.txt", ErrInvalidArchive)
	}
	if err := copyFile(archivePath, filepath.Join(content, "problem.zip"), 0o600); err != nil {
		return "", err
	}

	destination := filepath.Join(s.root, id)
	if err := os.RemoveAll(destination); err != nil {
		return "", err
	}
	if err := os.Rename(content, destination); err != nil {
		return "", err
	}
	return filepath.Join(destination, "settings.yaml"), nil
}

func (s *ProblemStore) OpenProblem(ctx context.Context, id string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateName(id); err != nil {
		return nil, err
	}
	archivePath := filepath.Join(s.root, id, "problem.zip")
	if file, err := os.Open(archivePath); err == nil {
		return file, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	content, err := zipDirectory(ctx, filepath.Join(s.root, id))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s *ProblemStore) DeleteProblem(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateName(id); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.root, id))
}

// SeedFixtures 將 Git 中的固定測試題庫複製到本機 runtime root（只補不存在的題目）。
func (s *ProblemStore) SeedFixtures(ctx context.Context, fixturesRoot string) ([]string, error) {
	entries, err := os.ReadDir(fixturesRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "113final") {
			continue
		}
		id := entry.Name()
		ids = append(ids, id)
		destination := filepath.Join(s.root, id)
		if _, err := os.Stat(destination); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if err := copyDirectory(ctx, filepath.Join(fixturesRoot, id), destination); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func copyDirectory(ctx context.Context, source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func zipDirectory(ctx context.Context, root string) ([]byte, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	w := zip.NewWriter(&output)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() || info.Name() == "problem.zip" {
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
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

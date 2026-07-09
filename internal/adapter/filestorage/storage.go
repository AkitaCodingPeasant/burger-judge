package filestorage

// 此檔案實作有限制的 ZIP 解壓縮與階段日誌持久化。

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"regs/internal/usecase"
)

const (
	maxArchiveSize  = 64 << 20
	maxArchiveFiles = 10_000
)

// ErrInvalidArchive 表示 ZIP 格式錯誤或包含不安全內容。
var ErrInvalidArchive = usecase.ErrInvalidSource

// Storage 將每筆提交保存在根目錄下經過驗證的子目錄。
type Storage struct {
	root string
}

// New 建立以檔案系統為後端的儲存服務。
func New(root string) *Storage { return &Storage{root: root} }

// SaveSource 儲存並安全解壓縮提交的 ZIP 檔。
func (s *Storage) SaveSource(ctx context.Context, id string, source io.Reader) error {
	if err := validateName(id); err != nil {
		return err
	}
	base := filepath.Join(s.root, id)
	if err := os.MkdirAll(base, 0o750); err != nil {
		return err
	}
	archivePath := filepath.Join(base, "source.zip")
	destination := filepath.Join(base, "source")
	complete := false
	// 若儲存或解壓縮失敗，移除不完整的檔案。
	defer func() {
		if !complete {
			_ = os.Remove(archivePath)
			_ = os.RemoveAll(destination)
		}
	}()
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(source, maxArchiveSize+1))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > maxArchiveSize {
		return fmt.Errorf("%w: archive exceeds %d bytes", ErrInvalidArchive, maxArchiveSize)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := extractZIP(ctx, archivePath, destination); err != nil {
		_ = os.RemoveAll(destination)
		return err
	}
	info, err := os.Stat(filepath.Join(destination, "CMakeLists.txt"))
	if err != nil || !info.Mode().IsRegular() {
		_ = os.RemoveAll(destination)
		return fmt.Errorf("%w: project root must contain CMakeLists.txt", ErrInvalidArchive)
	}
	complete = true
	return nil
}

// OpenSource 開啟提交時保存的原始 ZIP 檔。
func (s *Storage) OpenSource(ctx context.Context, id string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateName(id); err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(s.root, id, "source.zip"))
}

// SaveLog 建立或覆寫指定提交的階段日誌。
func (s *Storage) SaveLog(ctx context.Context, id, stage string, source io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.logPath(id, stage)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, source)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return closeErr
}

// OpenLog 開啟設定、編譯或執行階段的日誌。
func (s *Storage) OpenLog(ctx context.Context, id, stage string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.logPath(id, stage)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// SourceDir 回傳解壓縮原始碼的絕對路徑，供 Docker 掛載使用。
func (s *Storage) SourceDir(id string) (string, error) {
	if err := validateName(id); err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(s.root, id, "source"))
}

// logPath 驗證提交與階段名稱，並組合日誌檔路徑。
func (s *Storage) logPath(id, stage string) (string, error) {
	if err := validateName(id); err != nil {
		return "", err
	}
	switch stage {
	case "configure", "compile", "run":
	default:
		return "", fmt.Errorf("unknown log stage %q", stage)
	}
	return filepath.Join(s.root, id, "logs", stage+".log"), nil
}

// extractZIP 將 ZIP 安全解壓縮至目標目錄，並限制檔案數與總大小。
func extractZIP(ctx context.Context, archivePath, destination string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	defer r.Close()
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	if len(r.File) > maxArchiveFiles {
		return fmt.Errorf("%w: archive exceeds %d entries", ErrInvalidArchive, maxArchiveFiles)
	}
	cleanRoot := filepath.Clean(destination) + string(os.PathSeparator)
	var extracted int64
	for _, entry := range r.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name == "" || strings.ContainsRune(entry.Name, '\x00') || filepath.IsAbs(entry.Name) || filepath.VolumeName(entry.Name) != "" {
			return fmt.Errorf("%w: invalid archive path", ErrInvalidArchive)
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic links are not allowed", ErrInvalidArchive)
		}
		target := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanRoot) {
			return fmt.Errorf("%w: path escapes project root", ErrInvalidArchive)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if entry.UncompressedSize64 > maxArchiveSize || extracted > maxArchiveSize-int64(entry.UncompressedSize64) {
			return fmt.Errorf("%w: extracted content exceeds %d bytes", ErrInvalidArchive, maxArchiveSize)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		mode := entry.Mode().Perm() & 0o755
		if mode == 0 {
			mode = 0o640
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			in.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, maxArchiveSize-extracted+1))
		extracted += n
		inErr, outErr := in.Close(), out.Close()
		if copyErr != nil {
			return copyErr
		}
		if inErr != nil {
			return inErr
		}
		if outErr != nil {
			return outErr
		}
		if extracted > maxArchiveSize {
			return fmt.Errorf("%w: extracted content exceeds %d bytes", ErrInvalidArchive, maxArchiveSize)
		}
	}
	return nil
}

// validateName 驗證儲存鍵值只能是安全的單一路徑元件。
func validateName(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("invalid storage key %q", value)
	}
	return nil
}

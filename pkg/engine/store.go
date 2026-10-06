package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gofrs/flock"
	"github.com/natefinch/atomic"
)

type Store struct{ Root string }

func NewStore(root string) (*Store, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("locate application data directory: %w", err)
		}
		switch runtime.GOOS {
		case "darwin":
			root = filepath.Join(home, "Library", "Application Support", "supabase-toys")
		case "windows":
			base := os.Getenv("LOCALAPPDATA")
			if base == "" {
				base = filepath.Join(home, "AppData", "Local")
			}
			root = filepath.Join(base, "supabase-toys")
		default:
			base := os.Getenv("XDG_DATA_HOME")
			if base == "" {
				base = filepath.Join(home, ".local", "share")
			}
			root = filepath.Join(base, "supabase-toys")
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve registry path: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create registry directory: %w", err)
	}
	return &Store{Root: root}, nil
}
func (s *Store) Lock() (*flock.Flock, error) {
	lock := flock.New(filepath.Join(s.Root, "mutation.lock"))
	acquired, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock project registry: %w", err)
	}
	if !acquired {
		return nil, errors.New("another Supabase Toys operation is active; retry when it finishes")
	}
	return lock, nil
}
func (s *Store) Read() (Registry, error) {
	registry := Registry{Projects: []Project{}, Settings: Settings{SupabaseCLI: "supabase", DockerCLI: "docker"}}
	data, err := os.ReadFile(filepath.Join(s.Root, "registry.json"))
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, fmt.Errorf("read registry: %w", err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return registry, fmt.Errorf("invalid registry; preserve it and restore a backup: %w", err)
	}
	if registry.Projects == nil {
		registry.Projects = []Project{}
	}
	return registry, nil
}
func (s *Store) Write(registry Registry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode registry: %w", err)
	}
	return AtomicWrite(filepath.Join(s.Root, "registry.json"), data)
}
func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func AtomicWrite(path string, content []byte) error {
	if err := atomic.WriteFile(path, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("atomically write file: %w", err)
	}
	return nil
}
func backup(path string, content []byte) error {
	name := fmt.Sprintf("%s.toys-%d.bak", path, time.Now().UnixNano())
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create file backup: %w", err)
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}
func canonical(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project folder: %w", err)
	}
	result, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("project folder does not exist: %w", err)
	}
	return result, nil
}

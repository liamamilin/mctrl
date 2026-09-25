package project

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mctrl/internal/storage"
)

var ErrNotFound = errors.New("project not found")

type Project struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultRunner string `json:"default_runner"`
	CreatedAt     string `json:"created_at,omitempty"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

type file struct {
	SchemaVersion int       `json:"schema_version"`
	Projects      []Project `json:"projects"`
}

func NewStore(root string) *Store {
	return &Store{path: filepath.Join(root, "projects.json")}
}

func (s *Store) load() (file, error) {
	var result file
	result.SchemaVersion = 1
	if err := storage.ReadJSON(s.path, &result); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return file{SchemaVersion: 1, Projects: []Project{}}, nil
		}
		return file{}, err
	}
	if result.Projects == nil {
		result.Projects = []Project{}
	}
	if result.SchemaVersion != 1 {
		return file{}, fmt.Errorf("unsupported project schema version %d", result.SchemaVersion)
	}
	seen := make(map[string]struct{}, len(result.Projects))
	for _, item := range result.Projects {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Path) == "" {
			return file{}, fmt.Errorf("project registry contains an incomplete record")
		}
		if !validRunnerID(item.DefaultRunner) {
			return file{}, fmt.Errorf("project %q has unsupported default runner %q", item.ID, item.DefaultRunner)
		}
		if _, exists := seen[item.ID]; exists {
			return file{}, fmt.Errorf("project registry contains duplicate id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return result, nil
}

func (s *Store) save(value file) error {
	value.SchemaVersion = 1
	return storage.WriteJSONAtomic(s.path, value, 0600)
}

func (s *Store) mutate(fn func(*file) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return storage.WithLock(s.path, func() error {
		value, err := s.load()
		if err != nil {
			return err
		}
		if err := fn(&value); err != nil {
			return err
		}
		return s.save(value)
	})
}

func (s *Store) Validate() error {
	_, err := s.load()
	return err
}

func (s *Store) List() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.load()
	if err != nil {
		return nil, err
	}
	result := append([]Project(nil), value.Projects...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *Store) Get(id string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.load()
	if err != nil {
		return Project{}, err
	}
	for _, item := range value.Projects {
		if item.ID == id {
			return item, nil
		}
	}
	return Project{}, ErrNotFound
}

func (s *Store) Add(name, path, defaultRunner string) (Project, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(path) == "" {
		return Project{}, fmt.Errorf("project path is required")
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return Project{}, fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return Project{}, fmt.Errorf("project path: %w", err)
	}
	if !info.IsDir() {
		return Project{}, fmt.Errorf("project path is not a directory: %s", clean)
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(clean)
	}
	defaultRunner = strings.ToLower(strings.TrimSpace(defaultRunner))
	if defaultRunner == "" {
		defaultRunner = "shell"
	}
	if !validRunnerID(defaultRunner) {
		return Project{}, fmt.Errorf("unsupported default runner %q", defaultRunner)
	}
	var item Project
	err = s.mutate(func(value *file) error {
		for _, existing := range value.Projects {
			if existing.Path == clean {
				item = existing
				return nil
			}
		}
		item = Project{ID: slug(filepath.Base(clean)) + "-" + random(4), Name: name, Path: clean, DefaultRunner: defaultRunner, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		value.Projects = append(value.Projects, item)
		return nil
	})
	if err != nil {
		return Project{}, err
	}
	return item, nil
}

func (s *Store) Remove(id string) error {
	return s.mutate(func(value *file) error {
		for i, item := range value.Projects {
			if item.ID == id {
				value.Projects = append(value.Projects[:i], value.Projects[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

func validRunnerID(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "shell", "codex", "opencode":
		return true
	default:
		return false
	}
}

func slug(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "project"
	}
	if len(result) > 48 {
		result = result[:48]
	}
	return result
}

func random(n int) string {
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		return "local"
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"piccolod/internal/persistence"
	"piccolod/internal/remote"
	"piccolod/internal/state/paths"
)

type bootstrapRemoteStorage struct {
	repo persistence.RemoteRepo
	path string
	root string
}

func newBootstrapRemoteStorage(repo persistence.RemoteRepo, baseDir string) remote.Storage {
	if baseDir == "" {
		baseDir = paths.Root()
	}
	return &bootstrapRemoteStorage{
		repo: repo,
		path: filepath.Join(baseDir, "remote", "config.json"),
		root: baseDir,
	}
}

func (s *bootstrapRemoteStorage) Load(ctx context.Context) (remote.Config, error) {
	if s == nil {
		return remote.Config{}, errors.New("remote storage: unavailable")
	}
	if !s.isMounted() {
		if s.repo == nil {
			return remote.Config{}, remote.ErrLocked
		}
		repoCfg, err := s.repo.CurrentConfig(ctx)
		if err != nil {
			if errors.Is(err, persistence.ErrLocked) {
				return remote.Config{}, remote.ErrLocked
			}
			if errors.Is(err, persistence.ErrNotFound) {
				return remote.Config{}, nil
			}
			return remote.Config{}, err
		}
		if len(repoCfg.Payload) == 0 {
			return remote.Config{}, nil
		}
		var cfg remote.Config
		if err := json.Unmarshal(repoCfg.Payload, &cfg); err != nil {
			return remote.Config{}, err
		}
		return cfg, nil
	}
	data, err := os.ReadFile(s.path)
	if err == nil {
		var cfg remote.Config
		if parseErr := json.Unmarshal(data, &cfg); parseErr == nil {
			return cfg, nil
		} else {
			log.Printf("WARN: bootstrap remote config parse failed (%v); falling back to repo", parseErr)
			_ = os.Remove(s.path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return remote.Config{}, err
	}
	if s.repo == nil {
		return remote.Config{}, nil
	}
	repoCfg, err := s.repo.CurrentConfig(ctx)
	if err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			return remote.Config{}, remote.ErrLocked
		}
		if errors.Is(err, persistence.ErrNotFound) {
			return remote.Config{}, nil
		}
		return remote.Config{}, err
	}
	if len(repoCfg.Payload) == 0 {
		return remote.Config{}, nil
	}
	var cfg remote.Config
	if err := json.Unmarshal(repoCfg.Payload, &cfg); err != nil {
		return remote.Config{}, err
	}
	if err := writeAtomicJSON(s.path, repoCfg.Payload, 0o600); err != nil {
		log.Printf("WARN: failed to seed bootstrap remote config: %v", err)
	}
	return cfg, nil
}

func (s *bootstrapRemoteStorage) Save(ctx context.Context, cfg remote.Config) error {
	if s == nil {
		return errors.New("remote storage: unavailable")
	}
	if !s.isMounted() {
		return remote.ErrLocked
	}
	if cfg.DNSCredentials == nil {
		cfg.DNSCredentials = map[string]string{}
	}
	payload, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return err
	}
	if s.repo != nil {
		if err := s.repo.SaveConfig(ctx, persistence.RemoteConfig{Payload: payload}); err != nil {
			if errors.Is(err, persistence.ErrLocked) {
				return remote.ErrLocked
			}
			return err
		}
	}
	if err := writeAtomicJSON(s.path, payload, 0o600); err != nil {
		return err
	}
	return nil
}

func (s *bootstrapRemoteStorage) isMounted() bool {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(s.root, ".cipher")); err != nil {
		return false
	}
	return true
}

func writeAtomicJSON(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

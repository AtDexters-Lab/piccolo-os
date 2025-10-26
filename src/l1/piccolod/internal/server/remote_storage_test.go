package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"piccolod/internal/persistence"
	"piccolod/internal/remote"
)

type stubRemoteRepo struct {
	cfg  persistence.RemoteConfig
	err  error
	save []persistence.RemoteConfig
}

func (s *stubRemoteRepo) CurrentConfig(context.Context) (persistence.RemoteConfig, error) {
	return s.cfg, s.err
}

func (s *stubRemoteRepo) SaveConfig(_ context.Context, cfg persistence.RemoteConfig) error {
	s.save = append(s.save, cfg)
	return s.err
}

func TestBootstrapRemoteStorage_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
	repo := &stubRemoteRepo{}
	storage := newBootstrapRemoteStorage(repo, dir)

	want := remote.Config{Endpoint: "wss://nexus.example.com/connect"}
	data, _ := json.Marshal(want)
	path := filepath.Join(dir, "remote", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := storage.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Endpoint != want.Endpoint {
		t.Fatalf("expected %s, got %s", want.Endpoint, got.Endpoint)
	}
}

func TestBootstrapRemoteStorage_LoadFallbackRepo(t *testing.T) {
	dir := t.TempDir()
	want := remote.Config{Endpoint: "wss://nexus.example.com/connect"}
	payload, _ := json.Marshal(want)
	repo := &stubRemoteRepo{cfg: persistence.RemoteConfig{Payload: payload}}
	storage := newBootstrapRemoteStorage(repo, dir)

	got, err := storage.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Endpoint != want.Endpoint {
		t.Fatalf("expected %s, got %s", want.Endpoint, got.Endpoint)
	}
}

func TestBootstrapRemoteStorage_SaveWritesFileAndRepo(t *testing.T) {
	dir := t.TempDir()
	repo := &stubRemoteRepo{}
	storage := newBootstrapRemoteStorage(repo, dir)
	want := remote.Config{Endpoint: "wss://nexus.example.com/connect"}

	if err := storage.Save(context.Background(), want); err != nil {
		t.Fatalf("save: %v", err)
	}
	path := filepath.Join(dir, "remote", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var fromFile remote.Config
	if err := json.Unmarshal(data, &fromFile); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fromFile.Endpoint != want.Endpoint {
		t.Fatalf("expected %s, got %s", want.Endpoint, fromFile.Endpoint)
	}
	if len(repo.save) != 1 {
		t.Fatalf("expected repo save, got %d", len(repo.save))
	}
	var fromRepo remote.Config
	if err := json.Unmarshal(repo.save[0].Payload, &fromRepo); err != nil {
		t.Fatalf("unmarshal repo: %v", err)
	}
	if fromRepo.Endpoint != want.Endpoint {
		t.Fatalf("repo saved wrong data: %+v", fromRepo)
	}
}

func TestBootstrapRemoteStorage_SaveLocked(t *testing.T) {
	dir := t.TempDir()
	repo := &stubRemoteRepo{err: persistence.ErrLocked}
	storage := newBootstrapRemoteStorage(repo, dir)

	err := storage.Save(context.Background(), remote.Config{})
	if !errors.Is(err, remote.ErrLocked) {
		t.Fatalf("expected remote.ErrLocked, got %v", err)
	}
}

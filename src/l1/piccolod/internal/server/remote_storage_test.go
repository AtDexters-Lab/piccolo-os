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
	cfg       persistence.RemoteConfig
	err       error
	saveCalls int
	saved     persistence.RemoteConfig
}

func (s *stubRemoteRepo) CurrentConfig(context.Context) (persistence.RemoteConfig, error) {
	return s.cfg, s.err
}

func (s *stubRemoteRepo) SaveConfig(_ context.Context, cfg persistence.RemoteConfig) error {
	s.saveCalls++
	s.saved = persistence.RemoteConfig{Payload: append([]byte(nil), cfg.Payload...)}
	if s.err != nil {
		return s.err
	}
	s.cfg = s.saved
	return nil
}

func prepareBootstrapMount(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir mount: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cipher"), []byte("/ciphertext/bootstrap"), 0o600); err != nil {
		t.Fatalf("write cipher sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mode"), []byte("rw"), 0o600); err != nil {
		t.Fatalf("write mode sentinel: %v", err)
	}
}

func TestBootstrapRemoteStorage_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
	prepareBootstrapMount(t, dir)
	storage := newBootstrapRemoteStorage(nil, dir)

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
	prepareBootstrapMount(t, dir)
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
	// file should now exist
	if _, err := os.Stat(filepath.Join(dir, "remote", "config.json")); err != nil {
		t.Fatalf("expected bootstrap file seeded, stat err=%v", err)
	}
}

func TestBootstrapRemoteStorage_LoadCorruptedFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	prepareBootstrapMount(t, dir)
	path := filepath.Join(dir, "remote", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := remote.Config{Endpoint: "wss://bootstrap.example.com"}
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
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file reseeded, stat err=%v", err)
	}
}

func TestBootstrapRemoteStorage_SaveWritesFileAndRepo(t *testing.T) {
	dir := t.TempDir()
	prepareBootstrapMount(t, dir)
	storage := newBootstrapRemoteStorage(nil, dir)
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
}

func TestBootstrapRemoteStorage_SavePersistsRepo(t *testing.T) {
	dir := t.TempDir()
	prepareBootstrapMount(t, dir)
	repo := &stubRemoteRepo{}
	storage := newBootstrapRemoteStorage(repo, dir)
	want := remote.Config{
		Endpoint: "wss://nexus.example.com/connect",
		DNSCredentials: map[string]string{
			"provider": "token",
		},
	}

	if err := storage.Save(context.Background(), want); err != nil {
		t.Fatalf("save: %v", err)
	}
	if repo.saveCalls != 1 {
		t.Fatalf("expected repo save to be called once, got %d", repo.saveCalls)
	}
	var repoCfg remote.Config
	if err := json.Unmarshal(repo.saved.Payload, &repoCfg); err != nil {
		t.Fatalf("repo payload invalid json: %v", err)
	}
	if repoCfg.Endpoint != want.Endpoint {
		t.Fatalf("expected repo endpoint %s, got %s", want.Endpoint, repoCfg.Endpoint)
	}

	path := filepath.Join(dir, "remote", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) == "" {
		t.Fatalf("expected bootstrap file to be written")
	}
}

func TestBootstrapRemoteStorage_SaveRepoLocked(t *testing.T) {
	dir := t.TempDir()
	prepareBootstrapMount(t, dir)
	repo := &stubRemoteRepo{err: persistence.ErrLocked}
	storage := newBootstrapRemoteStorage(repo, dir)
	cfg := remote.Config{Endpoint: "wss://nexus.example.com/connect"}

	err := storage.Save(context.Background(), cfg)
	if !errors.Is(err, remote.ErrLocked) {
		t.Fatalf("expected remote.ErrLocked, got %v", err)
	}
	if repo.saveCalls != 1 {
		t.Fatalf("expected repo save to be attempted once, got %d", repo.saveCalls)
	}
	path := filepath.Join(dir, "remote", "config.json")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected bootstrap file absent on locked repo, stat err=%v", err)
	}
}

func TestBootstrapRemoteStorage_SaveMountNotReady(t *testing.T) {
	dir := t.TempDir()
	repo := &stubRemoteRepo{}
	storage := newBootstrapRemoteStorage(repo, dir)
	cfg := remote.Config{Endpoint: "wss://nexus.example.com/connect"}

	if err := storage.Save(context.Background(), cfg); !errors.Is(err, remote.ErrLocked) {
		t.Fatalf("expected remote.ErrLocked when mount missing, got %v", err)
	}
	if repo.saveCalls != 0 {
		t.Fatalf("expected repo save not attempted, got %d", repo.saveCalls)
	}
}

func TestBootstrapRemoteStorage_LoadMountNotReady(t *testing.T) {
	dir := t.TempDir()
	storage := newBootstrapRemoteStorage(nil, dir)

	if _, err := storage.Load(context.Background()); !errors.Is(err, remote.ErrLocked) {
		t.Fatalf("expected remote.ErrLocked when mount missing, got %v", err)
	}
}

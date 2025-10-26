package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"piccolod/internal/remote"
)

func TestBootstrapRemoteStorage_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
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

func TestBootstrapRemoteStorage_LoadCorruptedFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remote", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	storage := newBootstrapRemoteStorage(nil, dir)

	got, err := storage.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Endpoint != "" {
		t.Fatalf("expected empty endpoint, got %s", got.Endpoint)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected corrupted file removed, stat err=%v", err)
	}
}

func TestBootstrapRemoteStorage_SaveWritesFileAndRepo(t *testing.T) {
	dir := t.TempDir()
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

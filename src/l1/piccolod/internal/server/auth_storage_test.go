package server

import (
	"context"
	"errors"
	"testing"

	"piccolod/internal/auth"
	"piccolod/internal/persistence"
)

type fakeAuthRepo struct {
	initialized bool
	hash        string
	lastSaved   string
	setCalled   bool
	loadErr     error
	saveErr     error
	staleness   persistence.AuthStaleness
}

func (f *fakeAuthRepo) IsInitialized(ctx context.Context) (bool, error) {
	return f.initialized, f.loadErr
}

func (f *fakeAuthRepo) SetInitialized(ctx context.Context) error {
	f.setCalled = true
	if f.saveErr != nil {
		return f.saveErr
	}
	f.initialized = true
	return nil
}

func (f *fakeAuthRepo) PasswordHash(ctx context.Context) (string, error) {
	if f.loadErr != nil {
		return "", f.loadErr
	}
	return f.hash, nil
}

func (f *fakeAuthRepo) SavePasswordHash(ctx context.Context, hash string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.lastSaved = hash
	f.hash = hash
	return nil
}

func (f *fakeAuthRepo) Staleness(ctx context.Context) (persistence.AuthStaleness, error) {
	if f.loadErr != nil {
		return persistence.AuthStaleness{}, f.loadErr
	}
	return f.staleness, nil
}

func (f *fakeAuthRepo) UpdateStaleness(ctx context.Context, update persistence.AuthStalenessUpdate) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if update.PasswordStale != nil {
		f.staleness.PasswordStale = *update.PasswordStale
	}
	if update.PasswordStaleAt != nil {
		f.staleness.PasswordStaleAt = *update.PasswordStaleAt
	}
	if update.PasswordAckAt != nil {
		f.staleness.PasswordAckAt = *update.PasswordAckAt
	}
	if update.RecoveryStale != nil {
		f.staleness.RecoveryStale = *update.RecoveryStale
	}
	if update.RecoveryStaleAt != nil {
		f.staleness.RecoveryStaleAt = *update.RecoveryStaleAt
	}
	if update.RecoveryAckAt != nil {
		f.staleness.RecoveryAckAt = *update.RecoveryAckAt
	}
	return nil
}

func TestPersistenceAuthStorage_LoadAndSave(t *testing.T) {
	repo := &fakeAuthRepo{hash: "argon2", initialized: true}
	storage := newPersistenceAuthStorage(repo)
	state, err := storage.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !state.Initialized || state.PasswordHash != "argon2" {
		t.Fatalf("unexpected state: %#v", state)
	}
	state.PasswordHash = "argon2-new"
	if err := storage.Save(context.Background(), state); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.lastSaved != "argon2-new" {
		t.Fatalf("expected hash saved, got %q", repo.lastSaved)
	}
	if !repo.setCalled {
		t.Fatalf("expected SetInitialized to be called")
	}
}

func TestPersistenceAuthStorage_ErrorsPropagate(t *testing.T) {
	repo := &fakeAuthRepo{loadErr: persistence.ErrLocked}
	storage := newPersistenceAuthStorage(repo)
	if _, err := storage.Load(context.Background()); !errors.Is(err, persistence.ErrLocked) {
		t.Fatalf("expected ErrLocked from Load, got %v", err)
	}
	repo = &fakeAuthRepo{saveErr: errors.New("boom")}
	storage = newPersistenceAuthStorage(repo)
	if err := storage.Save(context.Background(), auth.State{Initialized: true}); err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom from Save, got %v", err)
	}
}

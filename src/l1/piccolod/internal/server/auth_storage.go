package server

import (
	"context"
	"errors"

	"piccolod/internal/auth"
	"piccolod/internal/persistence"
)

// persistenceAuthStorage implements auth.Storage using the encrypted control store.
type persistenceAuthStorage struct {
	repo persistence.AuthRepo
}

func newPersistenceAuthStorage(repo persistence.AuthRepo) auth.Storage {
	if repo == nil {
		return nil
	}
	return &persistenceAuthStorage{repo: repo}
}

func (s *persistenceAuthStorage) Load(ctx context.Context) (auth.State, error) {
	if s == nil || s.repo == nil {
		return auth.State{}, errors.New("auth storage: repo unavailable")
	}
	initialized, err := s.repo.IsInitialized(ctx)
	if err != nil {
		return auth.State{}, err
	}
	hash, err := s.repo.PasswordHash(ctx)
	if err != nil {
		return auth.State{}, err
	}
	return auth.State{Initialized: initialized, PasswordHash: hash}, nil
}

func (s *persistenceAuthStorage) Save(ctx context.Context, state auth.State) error {
	if s == nil || s.repo == nil {
		return errors.New("auth storage: repo unavailable")
	}
	if err := s.repo.SavePasswordHash(ctx, state.PasswordHash); err != nil {
		return err
	}
	if state.Initialized {
		if err := s.repo.SetInitialized(ctx); err != nil {
			return err
		}
	}
	return nil
}

package server

import (
	"context"
	"encoding/json"
	"errors"

	"piccolod/internal/persistence"
	"piccolod/internal/remote"
)

type persistenceRemoteStorage struct {
	repo persistence.RemoteRepo
}

func newPersistenceRemoteStorage(repo persistence.RemoteRepo) remote.Storage {
	if repo == nil {
		return nil
	}
	return &persistenceRemoteStorage{repo: repo}
}

func (s *persistenceRemoteStorage) Load(ctx context.Context) (remote.Config, error) {
	if s == nil || s.repo == nil {
		return remote.Config{}, errors.New("remote storage: repo unavailable")
	}
	cfg, err := s.repo.CurrentConfig(ctx)
	if err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			return remote.Config{}, remote.ErrLocked
		}
		if errors.Is(err, persistence.ErrNotFound) {
			return remote.Config{}, nil
		}
		return remote.Config{}, err
	}
	if len(cfg.Payload) == 0 {
		return remote.Config{}, nil
	}
	var out remote.Config
	if err := json.Unmarshal(cfg.Payload, &out); err != nil {
		return remote.Config{}, err
	}
	return out, nil
}

func (s *persistenceRemoteStorage) Save(ctx context.Context, cfg remote.Config) error {
	if s == nil || s.repo == nil {
		return errors.New("remote storage: repo unavailable")
	}
	payload, err := json.Marshal(&cfg)
	if err != nil {
		return err
	}
	if err := s.repo.SaveConfig(ctx, persistence.RemoteConfig{Payload: payload}); err != nil {
		if errors.Is(err, persistence.ErrLocked) {
			return remote.ErrLocked
		}
		return err
	}
	return nil
}

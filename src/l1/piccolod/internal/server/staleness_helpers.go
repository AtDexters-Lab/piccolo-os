package server

import (
	"context"
	"errors"
	"time"

	"piccolod/internal/persistence"
)

func (s *GinServer) authStalenessRepo() persistence.AuthRepo {
	if s == nil {
		return nil
	}
	if s.authRepo != nil {
		return s.authRepo
	}
	if s.persistence == nil {
		return nil
	}
	ctrl := s.persistence.Control()
	if ctrl == nil {
		return nil
	}
	return ctrl.Auth()
}

func (s *GinServer) readAuthStaleness(ctx context.Context) (persistence.AuthStaleness, error) {
	repo := s.authStalenessRepo()
	if repo == nil {
		return persistence.AuthStaleness{}, errors.New("auth repo unavailable")
	}
	return repo.Staleness(ctx)
}

func (s *GinServer) applyStalenessUpdate(ctx context.Context, update persistence.AuthStalenessUpdate) error {
	repo := s.authStalenessRepo()
	if repo == nil {
		return errors.New("auth repo unavailable")
	}
	return repo.UpdateStaleness(ctx, update)
}

func boolPtr(v bool) *bool {
	return &v
}

func timePtr(t time.Time) *time.Time {
	return &t
}

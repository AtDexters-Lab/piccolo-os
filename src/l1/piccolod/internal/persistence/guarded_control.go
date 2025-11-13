package persistence

import (
	"context"
	"time"
)

// guardedControlStore wraps a ControlStore and enforces kernel-leader-only
// semantics for mutating operations. Reads are always allowed when unlocked.
type guardedControlStore struct {
	inner    ControlStore
	lockable lockableControlStore
	revision interface {
		Revision(context.Context) (uint64, string, error)
	}
	health interface {
		QuickCheck(context.Context) (ControlHealthReport, error)
	}
	leader   func() bool
	onCommit func(context.Context)
}

func newGuardedControlStore(inner ControlStore, leader func() bool, onCommit func(context.Context)) ControlStore {
	if inner == nil || leader == nil {
		return inner
	}
	var lockable lockableControlStore
	if l, ok := inner.(lockableControlStore); ok {
		lockable = l
	}
	var rev interface {
		Revision(context.Context) (uint64, string, error)
	}
	if r, ok := inner.(interface {
		Revision(context.Context) (uint64, string, error)
	}); ok {
		rev = r
	}
	var health interface {
		QuickCheck(context.Context) (ControlHealthReport, error)
	}
	if h, ok := inner.(interface {
		QuickCheck(context.Context) (ControlHealthReport, error)
	}); ok {
		health = h
	}
	return &guardedControlStore{inner: inner, lockable: lockable, revision: rev, health: health, leader: leader, onCommit: onCommit}
}

func (g *guardedControlStore) Auth() AuthRepo {
	return &guardedAuthRepo{store: g, repo: g.inner.Auth()}
}
func (g *guardedControlStore) Remote() RemoteRepo {
	return &guardedRemoteRepo{store: g, repo: g.inner.Remote()}
}
func (g *guardedControlStore) AppState() AppStateRepo {
	return &guardedAppStateRepo{store: g, repo: g.inner.AppState()}
}
func (g *guardedControlStore) Close(ctx context.Context) error { return g.inner.Close(ctx) }

func (g *guardedControlStore) Lock() {
	if g.lockable != nil {
		g.lockable.Lock()
	}
}

func (g *guardedControlStore) Unlock(ctx context.Context) error {
	if g.lockable != nil {
		return g.lockable.Unlock(ctx)
	}
	return ErrNotImplemented
}

func (g *guardedControlStore) Revision(ctx context.Context) (uint64, string, error) {
	if g.revision == nil {
		return 0, "", ErrNotImplemented
	}
	return g.revision.Revision(ctx)
}

func (g *guardedControlStore) QuickCheck(ctx context.Context) (ControlHealthReport, error) {
	if g.health == nil {
		return ControlHealthReport{Status: ControlHealthStatusUnknown, Message: "quick check unavailable", CheckedAt: time.Now().UTC()}, nil
	}
	return g.health.QuickCheck(ctx)
}

func (g *guardedControlStore) notifyCommit(ctx context.Context, err error) error {
	if err == nil && g.onCommit != nil {
		g.onCommit(ctx)
	}
	return err
}

type guardedAuthRepo struct {
	store *guardedControlStore
	repo  AuthRepo
}

type guardedRemoteRepo struct {
	store *guardedControlStore
	repo  RemoteRepo
}

type guardedAppStateRepo struct {
	store *guardedControlStore
	repo  AppStateRepo
}

func (r *guardedAuthRepo) IsInitialized(ctx context.Context) (bool, error) {
	return r.repo.IsInitialized(ctx)
}
func (r *guardedAuthRepo) PasswordHash(ctx context.Context) (string, error) {
	return r.repo.PasswordHash(ctx)
}
func (r *guardedAuthRepo) Staleness(ctx context.Context) (AuthStaleness, error) {
	return r.repo.Staleness(ctx)
}

func (r *guardedAuthRepo) SetInitialized(ctx context.Context) error {
	if r.store.leader != nil && !r.store.leader() {
		return ErrNotLeader
	}
	return r.store.notifyCommit(ctx, r.repo.SetInitialized(ctx))
}

func (r *guardedAuthRepo) SavePasswordHash(ctx context.Context, hash string) error {
	if r.store.leader != nil && !r.store.leader() {
		return ErrNotLeader
	}
	return r.store.notifyCommit(ctx, r.repo.SavePasswordHash(ctx, hash))
}

func (r *guardedAuthRepo) UpdateStaleness(ctx context.Context, update AuthStalenessUpdate) error {
	if r.store.leader != nil && !r.store.leader() {
		return ErrNotLeader
	}
	return r.store.notifyCommit(ctx, r.repo.UpdateStaleness(ctx, update))
}

func (r *guardedRemoteRepo) CurrentConfig(ctx context.Context) (RemoteConfig, error) {
	return r.repo.CurrentConfig(ctx)
}

func (r *guardedRemoteRepo) SaveConfig(ctx context.Context, cfg RemoteConfig) error {
	if r.store.leader != nil && !r.store.leader() {
		return ErrNotLeader
	}
	return r.store.notifyCommit(ctx, r.repo.SaveConfig(ctx, cfg))
}

func (r *guardedAppStateRepo) ListApps(ctx context.Context) ([]AppRecord, error) {
	return r.repo.ListApps(ctx)
}

func (r *guardedAppStateRepo) UpsertApp(ctx context.Context, record AppRecord) error {
	if r.store.leader != nil && !r.store.leader() {
		return ErrNotLeader
	}
	return r.store.notifyCommit(ctx, r.repo.UpsertApp(ctx, record))
}

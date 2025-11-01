package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"

	"piccolod/internal/state/paths"
)

const (
	sqliteSchemaVersion       = 1
	controlPayloadVersion     = 1
	controlVolumeMetadataName = "piccolo.volume.json"
	gocryptfsConfigName       = "gocryptfs.conf"
)

const defaultCheckpointInterval = time.Minute

var defaultCheckpointFn = func(db *sql.DB) error {
	_, err := db.Exec(`PRAGMA wal_checkpoint(PASSIVE);`)
	return err
}

type controlPayload struct {
	Version         int           `json:"version"`
	AuthInitialized bool          `json:"auth_initialized"`
	Remote          *RemoteConfig `json:"remote,omitempty"`
	Apps            []AppRecord   `json:"apps,omitempty"`
	PasswordHash    string        `json:"password_hash,omitempty"`
	Revision        uint64        `json:"revision"`
	Checksum        string        `json:"checksum"`
}

type sqliteControlStore struct {
	mu                 sync.RWMutex
	db                 *sql.DB
	path               string
	mountDir           string
	cipherDir          string
	keySource          keyProvider
	loaded             bool
	readOnly           bool
	checkpointFn       func(*sql.DB) error
	checkpointInterval time.Duration
	lastCheckpoint     time.Time
	state              controlState
}

type keyProvider interface {
	WithSDEK(func([]byte) error) error
}

type controlState struct {
	authInitialized bool
	remoteConfig    *RemoteConfig
	apps            map[string]AppRecord
	passwordHash    string
	revision        uint64
	checksum        string
}

var detectReadOnlyMount = defaultReadOnlyDetector

func defaultReadOnlyDetector(path string) (bool, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return false, err
	}
	return st.Flags&unix.ST_RDONLY != 0, nil
}

func newControlStore(stateDir string, kp keyProvider) (ControlStore, error) {
	return newSQLiteControlStore(stateDir, kp)
}

func newSQLiteControlStore(stateDir string, kp keyProvider) (*sqliteControlStore, error) {
	if kp == nil {
		return nil, ErrCryptoUnavailable
	}
	base := stateDir
	if base == "" {
		base = paths.Root()
	}
	cipherDir := filepath.Join(base, "ciphertext", "control")
	if err := os.MkdirAll(cipherDir, 0o700); err != nil {
		return nil, err
	}
	mountDir := filepath.Join(base, "mounts", "control")
	if err := os.MkdirAll(mountDir, 0o700); err != nil {
		return nil, err
	}
	store := &sqliteControlStore{
		path:               filepath.Join(mountDir, "control.db"),
		mountDir:           mountDir,
		cipherDir:          cipherDir,
		keySource:          kp,
		checkpointFn:       defaultCheckpointFn,
		checkpointInterval: defaultCheckpointInterval,
		state: controlState{
			apps: make(map[string]AppRecord),
		},
	}
	return store, nil
}

func configureSQLite(db *sql.DB, readOnly bool) error {
	if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}
	if readOnly {
		if _, err := db.Exec(`PRAGMA query_only=1;`); err != nil {
			return fmt.Errorf("set query_only: %w", err)
		}
		return nil
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("set journal mode: %w", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=FULL;`); err != nil {
		return fmt.Errorf("set synchronous: %w", err)
	}
	return nil
}

func ensureControlVolumePrepared(dir string) error {
	required := []string{
		filepath.Join(dir, gocryptfsConfigName),
		filepath.Join(dir, controlVolumeMetadataName),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrLocked
			}
			return err
		}
	}
	return nil
}

func applyMigrations(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			revision INTEGER NOT NULL DEFAULT 0,
			checksum TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS auth_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			initialized INTEGER NOT NULL DEFAULT 0,
			password_hash TEXT,
			updated_at TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS remote_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			payload BLOB NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS apps (
			name TEXT PRIMARY KEY,
			payload BLOB NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`INSERT INTO meta (id, revision, checksum, updated_at)
			VALUES (1, 0, '', '')
			ON CONFLICT(id) DO NOTHING;`,
		`INSERT INTO auth_state (id, initialized, password_hash, updated_at)
			VALUES (1, 0, NULL, '')
			ON CONFLICT(id) DO NOTHING;`,
		`PRAGMA user_version=` + fmt.Sprint(sqliteSchemaVersion) + `;`,
	}
	for _, stmt := range stmts {
		if _, err = tx.Exec(stmt); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *sqliteControlStore) Close(ctx context.Context) error {
	_ = ctx
	s.Lock()
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *sqliteControlStore) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded = false
	s.state = controlState{apps: make(map[string]AppRecord)}
	s.readOnly = false
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
}

func (s *sqliteControlStore) Unlock(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	if s.keySource == nil {
		return ErrCryptoUnavailable
	}
	if err := s.keySource.WithSDEK(func([]byte) error { return nil }); err != nil {
		return err
	}
	if err := s.volumeReady(); err != nil {
		return err
	}
	if ro, err := detectReadOnlyMount(s.mountDir); err == nil {
		s.readOnly = ro
	} else {
		s.readOnly = false
	}
	if err := s.openDB(); err != nil {
		return err
	}
	state, err := s.loadState()
	if err != nil {
		return err
	}
	s.state = state
	s.loaded = true
	return nil
}

func (s *sqliteControlStore) loadState() (controlState, error) {
	state := controlState{
		apps: make(map[string]AppRecord),
	}
	if s.db == nil {
		if err := s.openDB(); err != nil {
			return state, err
		}
	}
	var (
		revision int64
		checksum string
		updated  string
	)
	err := s.db.QueryRow(`SELECT revision, checksum, updated_at FROM meta WHERE id=1`).Scan(&revision, &checksum, &updated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	state.revision = uint64(revision)
	state.checksum = checksum

	var (
		initInt      int
		passwordHash sql.NullString
		authUpdated  string
	)
	err = s.db.QueryRow(`SELECT initialized, password_hash, updated_at FROM auth_state WHERE id=1`).Scan(&initInt, &passwordHash, &authUpdated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	state.authInitialized = initInt == 1
	if passwordHash.Valid {
		state.passwordHash = passwordHash.String
	}

	var payload []byte
	var remoteUpdated string
	err = s.db.QueryRow(`SELECT payload, updated_at FROM remote_config WHERE id=1`).Scan(&payload, &remoteUpdated)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return state, err
		}
	} else {
		cfg := RemoteConfig{Payload: append([]byte{}, payload...)}
		state.remoteConfig = &cfg
	}

	rows, err := s.db.Query(`SELECT name, payload FROM apps`)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			name string
			data []byte
		)
		if err := rows.Scan(&name, &data); err != nil {
			return state, err
		}
		var record AppRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return state, err
		}
		state.apps[name] = record
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	return state, nil
}

func (s *sqliteControlStore) Revision(ctx context.Context) (uint64, string, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return 0, "", ErrLocked
	}
	return s.state.revision, s.state.checksum, nil
}

func (s *sqliteControlStore) QuickCheck(ctx context.Context) (ControlHealthReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if !s.loaded {
		return ControlHealthReport{Status: ControlHealthStatusUnknown, Message: "control store locked", CheckedAt: now}, nil
	}
	if err := s.openDB(); err != nil {
		return ControlHealthReport{Status: ControlHealthStatusError, Message: err.Error(), CheckedAt: now}, err
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA quick_check")
	if err != nil {
		return ControlHealthReport{Status: ControlHealthStatusError, Message: err.Error(), CheckedAt: now}, err
	}
	defer rows.Close()
	var issues []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return ControlHealthReport{Status: ControlHealthStatusError, Message: err.Error(), CheckedAt: now}, err
		}
		if strings.EqualFold(strings.TrimSpace(line), "ok") {
			continue
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			issues = append(issues, trimmed)
		}
	}
	if err := rows.Err(); err != nil {
		return ControlHealthReport{Status: ControlHealthStatusError, Message: err.Error(), CheckedAt: now}, err
	}
	if len(issues) == 0 {
		return ControlHealthReport{Status: ControlHealthStatusOK, Message: "ok", CheckedAt: now}, nil
	}
	return ControlHealthReport{Status: ControlHealthStatusDegraded, Message: strings.Join(issues, "; "), CheckedAt: now}, nil
}

func (s *sqliteControlStore) Auth() AuthRepo         { return &sqliteAuthRepo{store: s} }
func (s *sqliteControlStore) Remote() RemoteRepo     { return &sqliteRemoteRepo{store: s} }
func (s *sqliteControlStore) AppState() AppStateRepo { return &sqliteAppStateRepo{store: s} }

func (s *sqliteControlStore) ensureWritableLocked() error {
	if err := s.volumeReady(); err != nil {
		return err
	}
	if !s.loaded {
		return ErrLocked
	}
	if s.readOnly {
		return ErrLocked
	}
	return s.openDB()
}

func (s *sqliteControlStore) volumeReady() error {
	if err := ensureControlVolumePrepared(s.cipherDir); err != nil {
		return err
	}
	if os.Getenv("PICCOLO_ALLOW_UNMOUNTED_TESTS") != "1" {
		mounted, err := isMountPoint(s.mountDir)
		if err != nil {
			return err
		}
		if !mounted {
			return ErrLocked
		}
	}
	return nil
}

func (s *sqliteControlStore) withWrite(mutator func(tx *sql.Tx) error) error {
	if !s.loaded {
		return ErrLocked
	}
	if err := s.ensureWritableLocked(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := mutator(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	payload := controlPayload{
		Version:         controlPayloadVersion,
		AuthInitialized: s.state.authInitialized,
		PasswordHash:    s.state.passwordHash,
		Revision:        s.state.revision + 1,
	}
	if s.state.remoteConfig != nil {
		rc := cloneRemoteConfig(*s.state.remoteConfig)
		payload.Remote = &rc
	}
	if len(s.state.apps) > 0 {
		payload.Apps = make([]AppRecord, 0, len(s.state.apps))
		for _, app := range s.state.apps {
			payload.Apps = append(payload.Apps, app)
		}
		sort.Slice(payload.Apps, func(i, j int) bool { return payload.Apps[i].Name < payload.Apps[j].Name })
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(plain))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE meta SET revision=?, checksum=?, updated_at=? WHERE id=1`,
		payload.Revision, checksum, now); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.state.revision = payload.Revision
	s.state.checksum = checksum
	s.maybeCheckpointLocked()
	return nil
}

func (s *sqliteControlStore) updateAuthState(initialized bool, passwordHash *string) error {
	return s.withWrite(func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		hashValue := s.state.passwordHash
		if passwordHash != nil {
			hashValue = *passwordHash
		}
		if _, err := tx.Exec(`UPDATE auth_state SET initialized=?, password_hash=?, updated_at=? WHERE id=1`,
			boolToInt(initialized), hashValue, now); err != nil {
			return err
		}
		s.state.authInitialized = initialized
		if passwordHash != nil {
			s.state.passwordHash = *passwordHash
		}
		return nil
	})
}

func (s *sqliteControlStore) upsertRemoteConfig(payload []byte) error {
	return s.withWrite(func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`INSERT INTO remote_config (id, payload, updated_at) VALUES (1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
			payload, now); err != nil {
			return err
		}
		cfg := RemoteConfig{Payload: append([]byte{}, payload...)}
		s.state.remoteConfig = &cfg
		return nil
	})
}

func (s *sqliteControlStore) upsertApp(record AppRecord) error {
	return s.withWrite(func(tx *sql.Tx) error {
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`INSERT INTO apps (name, payload, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
			record.Name, data, now); err != nil {
			return err
		}
		s.state.apps[record.Name] = record
		return nil
	})
}

func (s *sqliteControlStore) maybeCheckpointLocked() {
	if s.db == nil || s.readOnly || s.checkpointFn == nil {
		return
	}
	if s.checkpointInterval > 0 && !s.lastCheckpoint.IsZero() {
		if time.Since(s.lastCheckpoint) < s.checkpointInterval {
			return
		}
	}
	if err := s.checkpointFn(s.db); err != nil {
		log.Printf("WARN: control-store checkpoint failed: %v", err)
		return
	}
	s.lastCheckpoint = time.Now().UTC()
}

func (s *sqliteControlStore) openDB() error {
	if s.db != nil {
		return nil
	}
	if s.readOnly {
		if _, err := os.Stat(s.path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrLocked
			}
			return err
		}
	}
	dsn := s.path
	if s.readOnly {
		dsn = buildSQLiteDSN(s.path, true)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	if err := configureSQLite(db, s.readOnly); err != nil {
		db.Close()
		return err
	}
	if !s.readOnly {
		if err := applyMigrations(db); err != nil {
			db.Close()
			return err
		}
	}
	s.db = db
	return nil
}

func cloneRemoteConfig(cfg RemoteConfig) RemoteConfig {
	dup := make([]byte, len(cfg.Payload))
	copy(dup, cfg.Payload)
	return RemoteConfig{Payload: dup}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func buildSQLiteDSN(path string, readOnly bool) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	u := &url.URL{Scheme: "file", Path: abs}
	if readOnly {
		u.RawQuery = "mode=ro"
	}
	return u.String()
}

var (
	_ ControlStore         = (*sqliteControlStore)(nil)
	_ lockableControlStore = (*sqliteControlStore)(nil)
)

type sqliteAuthRepo struct{ store *sqliteControlStore }

func (r *sqliteAuthRepo) IsInitialized(ctx context.Context) (bool, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if !r.store.loaded {
		return false, ErrLocked
	}
	return r.store.state.authInitialized, nil
}

func (r *sqliteAuthRepo) SetInitialized(ctx context.Context) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	if r.store.state.authInitialized {
		return nil
	}
	return r.store.updateAuthState(true, nil)
}

func (r *sqliteAuthRepo) PasswordHash(ctx context.Context) (string, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if !r.store.loaded {
		return "", ErrLocked
	}
	return r.store.state.passwordHash, nil
}

func (r *sqliteAuthRepo) SavePasswordHash(ctx context.Context, hash string) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	return r.store.updateAuthState(r.store.state.authInitialized, &hash)
}

type sqliteRemoteRepo struct{ store *sqliteControlStore }

func (r *sqliteRemoteRepo) CurrentConfig(ctx context.Context) (RemoteConfig, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if !r.store.loaded {
		return RemoteConfig{}, ErrLocked
	}
	if r.store.state.remoteConfig == nil {
		return RemoteConfig{}, ErrNotFound
	}
	return cloneRemoteConfig(*r.store.state.remoteConfig), nil
}

func (r *sqliteRemoteRepo) SaveConfig(ctx context.Context, cfg RemoteConfig) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	copyCfg := cloneRemoteConfig(cfg)
	return r.store.upsertRemoteConfig(copyCfg.Payload)
}

type sqliteAppStateRepo struct{ store *sqliteControlStore }

func (r *sqliteAppStateRepo) ListApps(ctx context.Context) ([]AppRecord, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if !r.store.loaded {
		return nil, ErrLocked
	}
	result := make([]AppRecord, 0, len(r.store.state.apps))
	for _, app := range r.store.state.apps {
		result = append(result, app)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *sqliteAppStateRepo) UpsertApp(ctx context.Context, record AppRecord) error {
	if record.Name == "" {
		return errors.New("app name required")
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	return r.store.upsertApp(record)
}

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// State captures the persisted authentication metadata.
type State struct {
	Initialized  bool
	PasswordHash string
}

// Storage abstracts the persistence backend for auth state.
type Storage interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, state State) error
}

// Manager stores and verifies the admin credentials.
// For v1 we support a single local admin user: "admin".
type Manager struct {
	storage Storage
	mu      sync.RWMutex
	state   State
	loaded  bool
}

// NewManager constructs a manager that persists state to the given directory.
// This legacy constructor is retained for tests; production code should prefer
// NewManagerWithStorage with a persistence-backed storage layer.
func NewManager(stateDir string) (*Manager, error) {
	storage, err := newFilesystemStorage(stateDir)
	if err != nil {
		return nil, err
	}
	return NewManagerWithStorage(storage)
}

// NewManagerWithStorage constructs a manager with the provided storage backend.
func NewManagerWithStorage(storage Storage) (*Manager, error) {
	if storage == nil {
		return nil, errors.New("auth: storage required")
	}
	return &Manager{storage: storage}, nil
}

func (m *Manager) ensureLoaded(ctx context.Context) error {
	if m.loaded {
		return nil
	}
	state, err := m.storage.Load(ctx)
	if err != nil {
		return err
	}
	m.state = state
	m.loaded = true
	return nil
}

func (m *Manager) getState(ctx context.Context) (State, error) {
	m.mu.RLock()
	if m.loaded {
		st := m.state
		m.mu.RUnlock()
		return st, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoaded(ctx); err != nil {
		return State{}, err
	}
	return m.state, nil
}

func (m *Manager) updateState(ctx context.Context, fn func(*State) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoaded(ctx); err != nil {
		return err
	}
	if err := fn(&m.state); err != nil {
		return err
	}
	if err := m.storage.Save(ctx, m.state); err != nil {
		return err
	}
	return nil
}

// IsInitialized returns true if admin has been set up.
func (m *Manager) IsInitialized(ctx context.Context) (bool, error) {
	st, err := m.getState(ctx)
	if err != nil {
		return false, err
	}
	return st.Initialized, nil
}

// Setup initializes the admin password; allowed only once.
func (m *Manager) Setup(ctx context.Context, password string) error {
	if strings.TrimSpace(password) == "" {
		return errors.New("password required")
	}
	return m.updateState(ctx, func(state *State) error {
		if state.Initialized {
			return errors.New("admin already set up")
		}
		ref, err := hashArgon2id(password)
		if err != nil {
			return err
		}
		state.Initialized = true
		state.PasswordHash = ref
		return nil
	})
}

// ChangePassword changes the admin password after verifying the old one.
func (m *Manager) ChangePassword(ctx context.Context, old, newp string) error {
	if strings.TrimSpace(old) == "" || strings.TrimSpace(newp) == "" {
		return errors.New("passwords required")
	}
	return m.updateState(ctx, func(state *State) error {
		if !state.Initialized {
			return errors.New("not initialized")
		}
		if !verifyArgon2id(state.PasswordHash, old) {
			return errors.New("invalid credentials")
		}
		ref, err := hashArgon2id(newp)
		if err != nil {
			return err
		}
		state.PasswordHash = ref
		return nil
	})
}

// ChangePasswordWithRecovery bypasses verification for recovery-key scenarios.
func (m *Manager) ChangePasswordWithRecovery(ctx context.Context, newp string) error {
	if strings.TrimSpace(newp) == "" {
		return errors.New("password required")
	}
	return m.updateState(ctx, func(state *State) error {
		if !state.Initialized {
			return errors.New("not initialized")
		}
		ref, err := hashArgon2id(newp)
		if err != nil {
			return err
		}
		state.PasswordHash = ref
		return nil
	})
}

// Verify returns true if (username=="admin" && password valid).
func (m *Manager) Verify(ctx context.Context, username, password string) (bool, error) {
	if username != "admin" {
		return false, nil
	}
	st, err := m.getState(ctx)
	if err != nil {
		return false, err
	}
	if !st.Initialized {
		return false, nil
	}
	return verifyArgon2id(st.PasswordHash, password), nil
}

type fileState struct {
	Initialized bool   `json:"initialized"`
	Password    string `json:"password_hash"`
}

type filesystemStorage struct {
	path string
}

func newFilesystemStorage(stateDir string) (*filesystemStorage, error) {
	if stateDir == "" {
		stateDir = "/tmp/piccolo"
	}
	dir := filepath.Join(stateDir, "auth")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &filesystemStorage{path: filepath.Join(dir, "admin.json")}, nil
}

func (s *filesystemStorage) Load(ctx context.Context) (State, error) {
	_ = ctx
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var fs fileState
	if err := json.Unmarshal(data, &fs); err != nil {
		return State{}, err
	}
	state := State{Initialized: fs.Initialized, PasswordHash: fs.Password}
	if !state.Initialized && state.PasswordHash != "" {
		state.Initialized = true
	}
	return state, nil
}

func (s *filesystemStorage) Save(ctx context.Context, state State) error {
	_ = ctx
	fs := fileState{Initialized: state.Initialized, Password: state.PasswordHash}
	data, err := json.MarshalIndent(&fs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// Argon2id helpers (simple encoded format: argon2id$v=19$m=...,t=...,p=...$saltB64$hashB64)
func hashArgon2id(password string) (string, error) {
	// Soft profile defaults
	var (
		time    uint32 = 3
		memory  uint32 = 64 * 1024 // 64MB
		keyLen  uint32 = 32
		saltLen        = 16
	)
	threads := uint8(selectAuthParallelism())
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, time, threads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func selectAuthParallelism() int {
	cores := runtime.NumCPU()
	if cores <= 1 {
		return 1
	}
	p := cores / 2
	if p < 1 {
		p = 1
	}
	if p > 4 {
		p = 4
	}
	return p
}

func verifyArgon2id(encoded, password string) bool {
	// Expected tokens: ["argon2id", "v=19", "m=...,t=...,p=...", "saltB64", "hashB64"]
	toks := strings.Split(encoded, "$")
	if len(toks) < 5 || toks[0] != "argon2id" {
		return false
	}
	// Parse parameters
	var memory, timeIters, threads uint64
	params := strings.Split(toks[2], ",")
	for _, p := range params {
		if !strings.Contains(p, "=") {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		switch kv[0] {
		case "m":
			v, _ := strconv.ParseUint(kv[1], 10, 32)
			memory = v
		case "t":
			v, _ := strconv.ParseUint(kv[1], 10, 32)
			timeIters = v
		case "p":
			v, _ := strconv.ParseUint(kv[1], 10, 8)
			threads = v
		}
	}
	salt, err := base64.RawStdEncoding.DecodeString(toks[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(toks[4])
	if err != nil {
		return false
	}
	calc := argon2.IDKey([]byte(password), salt, uint32(timeIters), uint32(memory), uint8(threads), uint32(len(want)))
	if len(calc) != len(want) {
		return false
	}
	var v byte
	for i := range calc {
		v |= calc[i] ^ want[i]
	}
	return v == 0
}

// Session store (in-memory)
type Session struct {
	ID        string
	User      string
	CSRF      string
	ExpiresAt int64 // unix seconds
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *SessionStore) Create(user string, ttlSeconds int64) *Session {
	id := randString(32)
	csrf := randString(16)
	sess := &Session{ID: id, User: user, CSRF: csrf, ExpiresAt: (timeNow().Unix() + ttlSeconds)}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if timeNow().Unix() > sess.ExpiresAt {
		s.Delete(id)
		return nil, false
	}
	return sess, true
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *SessionStore) RotateCSRF(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return "", false
	}
	sess.CSRF = randString(16)
	return sess.CSRF, true
}

// timeNow is a small indirection for tests
var timeNow = func() time.Time { return time.Now() }

package auth

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "golang.org/x/crypto/argon2"
)

// Manager stores and verifies the admin credentials.
// For v1 we support a single local admin user: "admin".
type Manager struct {
    path        string
    mu          sync.RWMutex
    passwordRef string // encoded argon2id string
}

type fileState struct {
    Password string `json:"password_hash"`
}

func NewManager(stateDir string) (*Manager, error) {
    if stateDir == "" {
        stateDir = "/tmp/piccolo"
    }
    if err := os.MkdirAll(filepath.Join(stateDir, "auth"), 0o700); err != nil {
        return nil, err
    }
    m := &Manager{path: filepath.Join(stateDir, "auth", "admin.json")}
    _ = m.load()
    return m, nil
}

func (m *Manager) load() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    b, err := os.ReadFile(m.path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    var st fileState
    if err := json.Unmarshal(b, &st); err != nil {
        return err
    }
    m.passwordRef = st.Password
    return nil
}

func (m *Manager) save() error {
    st := fileState{Password: m.passwordRef}
    b, _ := json.MarshalIndent(&st, "", "  ")
    return os.WriteFile(m.path, b, 0o600)
}

// IsInitialized returns true if admin has been set up.
func (m *Manager) IsInitialized() bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.passwordRef != ""
}

// Setup initializes the admin password; allowed only once.
func (m *Manager) Setup(password string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.passwordRef != "" {
        return errors.New("admin already set up")
    }
    ref, err := hashArgon2id(password)
    if err != nil {
        return err
    }
    m.passwordRef = ref
    return m.save()
}

// ChangePassword changes the admin password after verifying the old one.
func (m *Manager) ChangePassword(old, newp string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.passwordRef == "" {
        return errors.New("not initialized")
    }
    if !verifyArgon2id(m.passwordRef, old) {
        return errors.New("invalid credentials")
    }
    ref, err := hashArgon2id(newp)
    if err != nil {
        return err
    }
    m.passwordRef = ref
    return m.save()
}

// Verify returns true if (username=="admin" && password valid).
func (m *Manager) Verify(username, password string) bool {
    if username != "admin" {
        return false
    }
    m.mu.RLock()
    ref := m.passwordRef
    m.mu.RUnlock()
    if ref == "" {
        return false
    }
    return verifyArgon2id(ref, password)
}

// Argon2id helpers (simple encoded format: argon2id$v=19$m=...,t=...,p=...$saltB64$hashB64)
func hashArgon2id(password string) (string, error) {
    // Parameters (moderate defaults suitable for small devices)
    var (
        time    uint32 = 3
        memory  uint32 = 64 * 1024 // 64MB
        threads uint8  = 1
        keyLen  uint32 = 32
        saltLen        = 16
    )
    salt := make([]byte, saltLen)
    if _, err := rand.Read(salt); err != nil {
        return "", err
    }
    hash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
    return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, time, threads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyArgon2id(encoded, password string) bool {
    // Parse
    // Expected: argon2id$v=19$m=...,t=...,p=...$salt$hash
    var memory uint32
    var time uint32
    var threads uint8
    var saltB64, hashB64 string
    n, err := fmt.Sscanf(encoded, "argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", &memory, &time, &threads, &saltB64, &hashB64)
    if n != 5 || err != nil {
        return false
    }
    salt, err := base64.RawStdEncoding.DecodeString(saltB64)
    if err != nil { return false }
    want, err := base64.RawStdEncoding.DecodeString(hashB64)
    if err != nil { return false }
    calc := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
    if len(calc) != len(want) { return false }
    // Constant time compare
    var v byte
    for i := range calc { v |= calc[i] ^ want[i] }
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
    if !ok { return nil, false }
    if timeNow().Unix() > sess.ExpiresAt {
        s.Delete(id)
        return nil, false
    }
    return sess, true
}

func (s *SessionStore) Delete(id string) {
    s.mu.Lock(); delete(s.sessions, id); s.mu.Unlock()
}

func (s *SessionStore) RotateCSRF(id string) (string, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()
    sess, ok := s.sessions[id]
    if !ok { return "", false }
    sess.CSRF = randString(16)
    return sess.CSRF, true
}

// timeNow is a small indirection for tests
var timeNow = func() time.Time { return time.Now() }

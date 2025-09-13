package crypt

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sync"

    "golang.org/x/crypto/argon2"
)

type kdfParams struct {
    Alg     string `json:"alg"`
    Time    uint32 `json:"time"`
    Memory  uint32 `json:"memory"`
    Threads uint8  `json:"threads"`
}

type fileState struct {
    SDEK  string    `json:"sdek"`  // base64 ciphertext
    Salt  string    `json:"salt"`  // base64
    Nonce string    `json:"nonce"` // base64
    KDF   kdfParams `json:"kdf"`
}

// Manager controls encryption key setup and unlock lifecycle.
// It intentionally does not manage any mounts; it only keeps the SDEK in memory when unlocked.
type Manager struct {
    path   string
    mu     sync.RWMutex
    sdek   []byte // plaintext SDEK when unlocked
    inited bool
}

func NewManager(stateDir string) (*Manager, error) {
    if stateDir == "" { stateDir = "/var/lib/piccolod" }
    dir := filepath.Join(stateDir, "crypto")
    if err := os.MkdirAll(dir, 0o700); err != nil { return nil, err }
    m := &Manager{ path: filepath.Join(dir, "keyset.json") }
    if _, err := os.Stat(m.path); err == nil { m.inited = true } else { m.inited = false }
    return m, nil
}

func (m *Manager) IsInitialized() bool {
    m.mu.RLock(); defer m.mu.RUnlock()
    return m.inited
}

func (m *Manager) IsLocked() bool {
    m.mu.RLock(); defer m.mu.RUnlock()
    if !m.inited { return false }
    return len(m.sdek) == 0
}

func (m *Manager) deriveKey(pw string, salt []byte, k kdfParams) []byte {
    return argon2.IDKey([]byte(pw), salt, k.Time, k.Memory, k.Threads, 32)
}

func (m *Manager) Setup(password string) error {
    if password == "" { return errors.New("password required") }
    m.mu.Lock(); defer m.mu.Unlock()
    if m.inited { return errors.New("already initialized") }

    // KDF defaults (moderate)
    params := kdfParams{ Alg: "argon2id", Time: 3, Memory: 64*1024, Threads: 1 }
    salt := make([]byte, 16)
    if _, err := rand.Read(salt); err != nil { return err }
    key := m.deriveKey(password, salt, params)

    // Generate SDEK and seal with AES-GCM
    sdek := make([]byte, 32)
    if _, err := rand.Read(sdek); err != nil { return err }
    block, err := aes.NewCipher(key)
    if err != nil { return err }
    aead, err := cipher.NewGCM(block)
    if err != nil { return err }
    nonce := make([]byte, aead.NonceSize())
    if _, err := rand.Read(nonce); err != nil { return err }
    ct := aead.Seal(nil, nonce, sdek, nil)

    st := fileState{
        SDEK:  base64.RawStdEncoding.EncodeToString(ct),
        Salt:  base64.RawStdEncoding.EncodeToString(salt),
        Nonce: base64.RawStdEncoding.EncodeToString(nonce),
        KDF:   params,
    }
    b, _ := json.MarshalIndent(&st, "", "  ")
    if err := os.WriteFile(m.path, b, 0o600); err != nil { return err }
    m.inited = true
    m.sdek = nil // locked by default after setup
    return nil
}

func (m *Manager) Unlock(password string) error {
    if password == "" { return errors.New("password required") }
    m.mu.Lock(); defer m.mu.Unlock()
    if !m.inited { return errors.New("not initialized") }
    b, err := os.ReadFile(m.path)
    if err != nil { return err }
    var st fileState
    if err := json.Unmarshal(b, &st); err != nil { return err }
    if st.KDF.Alg != "argon2id" { return fmt.Errorf("unsupported kdf: %s", st.KDF.Alg) }
    salt, err := base64.RawStdEncoding.DecodeString(st.Salt); if err != nil { return err }
    key := m.deriveKey(password, salt, st.KDF)
    block, err := aes.NewCipher(key); if err != nil { return err }
    aead, err := cipher.NewGCM(block); if err != nil { return err }
    nonce, err := base64.RawStdEncoding.DecodeString(st.Nonce); if err != nil { return err }
    ct, err := base64.RawStdEncoding.DecodeString(st.SDEK); if err != nil { return err }
    pt, err := aead.Open(nil, nonce, ct, nil)
    if err != nil { return errors.New("invalid password") }
    m.sdek = pt
    return nil
}

func (m *Manager) Lock() {
    m.mu.Lock(); defer m.mu.Unlock()
    // Zero sdek
    for i := range m.sdek { m.sdek[i] = 0 }
    m.sdek = nil
}


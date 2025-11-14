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
	"runtime"
	"sync"

	"piccolod/internal/state/paths"

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
	// Optional recovery-key wrapped SDEK
	RKSalt  string `json:"rk_salt,omitempty"`
	RKNonce string `json:"rk_nonce,omitempty"`
	SDEKRK  string `json:"sdek_rk,omitempty"`
}

// Manager controls encryption key setup and unlock lifecycle.
// It intentionally does not manage any mounts; it only keeps the SDEK in memory when unlocked.
type Manager struct {
	path   string
	mu     sync.RWMutex
	sdek   []byte // plaintext SDEK when unlocked
	inited bool
}

var (
	ErrNotInitialized = errors.New("crypt: not initialized")
	ErrLocked         = errors.New("crypt: locked")
)

func NewManager(stateDir string) (*Manager, error) {
	if stateDir == "" {
		stateDir = paths.Root()
	}
	dir := filepath.Join(stateDir, "crypto")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{path: filepath.Join(dir, "keyset.json")}
	if _, err := os.Stat(m.path); err == nil {
		m.inited = true
	} else {
		m.inited = false
	}
	return m, nil
}

func (m *Manager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.inited
}

func (m *Manager) IsLocked() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.inited {
		return false
	}
	return len(m.sdek) == 0
}

// WithSDEK invokes fn with a copy of the unlocked SDEK. Returns ErrNotInitialized
// if setup has not run, or ErrLocked when the manager is locked.
func (m *Manager) WithSDEK(fn func([]byte) error) error {
	if fn == nil {
		return errors.New("callback required")
	}
	m.mu.RLock()
	if !m.inited {
		m.mu.RUnlock()
		return ErrNotInitialized
	}
	if len(m.sdek) == 0 {
		m.mu.RUnlock()
		return ErrLocked
	}
	buf := make([]byte, len(m.sdek))
	copy(buf, m.sdek)
	m.mu.RUnlock()
	defer zeroBytes(buf)
	return fn(buf)
}

func (m *Manager) deriveKey(pw string, salt []byte, k kdfParams) []byte {
	return argon2.IDKey([]byte(pw), salt, k.Time, k.Memory, k.Threads, 32)
}

func (m *Manager) Setup(password string) error {
	if password == "" {
		return errors.New("password required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inited {
		return errors.New("already initialized")
	}

	// KDF defaults (hard profile)
	params := kdfParams{Alg: "argon2id", Time: 3, Memory: 512 * 1024, Threads: uint8(selectCryptoParallelism())}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := m.deriveKey(password, salt, params)

	// Generate SDEK and seal with AES-GCM
	sdek := make([]byte, 32)
	if _, err := rand.Read(sdek); err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := aead.Seal(nil, nonce, sdek, nil)

	st := fileState{
		SDEK:  base64.RawStdEncoding.EncodeToString(ct),
		Salt:  base64.RawStdEncoding.EncodeToString(salt),
		Nonce: base64.RawStdEncoding.EncodeToString(nonce),
		KDF:   params,
	}
	b, _ := json.MarshalIndent(&st, "", "  ")
	if err := os.WriteFile(m.path, b, 0o600); err != nil {
		return err
	}
	m.inited = true
	m.sdek = nil // locked by default after setup
	return nil
}

func (m *Manager) Unlock(password string) error {
	if password == "" {
		return errors.New("password required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.inited {
		return errors.New("not initialized")
	}
	b, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.KDF.Alg != "argon2id" {
		return fmt.Errorf("unsupported kdf: %s", st.KDF.Alg)
	}
	salt, err := base64.RawStdEncoding.DecodeString(st.Salt)
	if err != nil {
		return err
	}
	key := m.deriveKey(password, salt, st.KDF)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce, err := base64.RawStdEncoding.DecodeString(st.Nonce)
	if err != nil {
		return err
	}
	ct, err := base64.RawStdEncoding.DecodeString(st.SDEK)
	if err != nil {
		return err
	}
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return errors.New("invalid password")
	}
	m.sdek = pt
	return nil
}

func (m *Manager) Lock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Zero sdek
	for i := range m.sdek {
		m.sdek[i] = 0
	}
	m.sdek = nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func selectCryptoParallelism() int {
	cores := runtime.NumCPU()
	if cores <= 1 {
		return 1
	}
	p := cores - 1
	if p < 1 {
		p = 1
	}
	if p > 8 {
		p = 8
	}
	return p
}

// Rewrap decrypts SDEK with old password and re-seals it with new password.
func (m *Manager) Rewrap(oldPassword, newPassword string) error {
	if oldPassword == "" || newPassword == "" {
		return errors.New("passwords required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.inited {
		return errors.New("not initialized")
	}
	b, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.KDF.Alg != "argon2id" {
		return fmt.Errorf("unsupported kdf: %s", st.KDF.Alg)
	}
	salt, _ := base64.RawStdEncoding.DecodeString(st.Salt)
	keyOld := m.deriveKey(oldPassword, salt, st.KDF)
	block, _ := aes.NewCipher(keyOld)
	aead, _ := cipher.NewGCM(block)
	nonce, _ := base64.RawStdEncoding.DecodeString(st.Nonce)
	ct, _ := base64.RawStdEncoding.DecodeString(st.SDEK)
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return errors.New("invalid old password")
	}
	// Derive new key with new salt
	newSalt := make([]byte, 16)
	if _, err := rand.Read(newSalt); err != nil {
		return err
	}
	keyNew := m.deriveKey(newPassword, newSalt, st.KDF)
	block2, _ := aes.NewCipher(keyNew)
	aead2, _ := cipher.NewGCM(block2)
	newNonce := make([]byte, aead2.NonceSize())
	if _, err := rand.Read(newNonce); err != nil {
		return err
	}
	newCT := aead2.Seal(nil, newNonce, pt, nil)
	st.SDEK = base64.RawStdEncoding.EncodeToString(newCT)
	st.Salt = base64.RawStdEncoding.EncodeToString(newSalt)
	st.Nonce = base64.RawStdEncoding.EncodeToString(newNonce)
	// Save
	nb, _ := json.MarshalIndent(&st, "", "  ")
	if err := os.WriteFile(m.path, nb, 0o600); err != nil {
		return err
	}
	return nil
}

// RewrapUnlocked reseals the in-memory SDEK with a new password without
// requiring the previous password, assuming the manager is currently unlocked.
func (m *Manager) RewrapUnlocked(newPassword string) error {
	if newPassword == "" {
		return errors.New("password required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.inited {
		return errors.New("not initialized")
	}
	if len(m.sdek) == 0 {
		return ErrLocked
	}
	b, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.KDF.Alg != "argon2id" {
		return fmt.Errorf("unsupported kdf: %s", st.KDF.Alg)
	}
	newSalt := make([]byte, 16)
	if _, err := rand.Read(newSalt); err != nil {
		return err
	}
	keyNew := m.deriveKey(newPassword, newSalt, st.KDF)
	block, err := aes.NewCipher(keyNew)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	newNonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(newNonce); err != nil {
		return err
	}
	newCT := aead.Seal(nil, newNonce, m.sdek, nil)
	st.SDEK = base64.RawStdEncoding.EncodeToString(newCT)
	st.Salt = base64.RawStdEncoding.EncodeToString(newSalt)
	st.Nonce = base64.RawStdEncoding.EncodeToString(newNonce)
	nb, err := json.MarshalIndent(&st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, nb, 0o600)
}

// Recovery key management
var wordlist = []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet", "kilo", "lima", "mike", "november", "oscar", "papa", "quebec", "romeo", "sierra", "tango", "uniform", "victor", "whiskey", "xray", "yankee", "zulu"}

func (m *Manager) GenerateRecoveryKey(force bool) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.inited {
		return nil, errors.New("not initialized")
	}
	b, err := os.ReadFile(m.path)
	if err != nil {
		return nil, err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st.SDEKRK != "" && !force {
		return nil, errors.New("recovery key already set")
	}
	// Use plaintext SDEK if already unlocked; otherwise require password via helper
	var sdek []byte
	if len(m.sdek) > 0 {
		sdek = make([]byte, len(m.sdek))
		copy(sdek, m.sdek)
	} else {
		return nil, errors.New("unlock required")
	}
	// Generate 24-word mnemonic
	words := make([]string, 24)
	rb := make([]byte, 24)
	if _, err := rand.Read(rb); err != nil {
		return nil, err
	}
	for i := 0; i < 24; i++ {
		words[i] = wordlist[int(rb[i])%len(wordlist)]
	}
	mnemonic := ""
	for i, w := range words {
		if i > 0 {
			mnemonic += " "
		}
		mnemonic += w
	}
	// Derive RK key from mnemonic with new salt
	rkSalt := make([]byte, 16)
	if _, err := rand.Read(rkSalt); err != nil {
		return nil, err
	}
	rkParams := st.KDF
	rkKey := m.deriveKey(mnemonic, rkSalt, rkParams)
	block, _ := aes.NewCipher(rkKey)
	aead, _ := cipher.NewGCM(block)
	rkNonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(rkNonce); err != nil {
		return nil, err
	}
	rkCT := aead.Seal(nil, rkNonce, sdek, nil)
	st.RKSalt = base64.RawStdEncoding.EncodeToString(rkSalt)
	st.RKNonce = base64.RawStdEncoding.EncodeToString(rkNonce)
	st.SDEKRK = base64.RawStdEncoding.EncodeToString(rkCT)
	// Save
	nb, _ := json.MarshalIndent(&st, "", "  ")
	if err := os.WriteFile(m.path, nb, 0o600); err != nil {
		return nil, err
	}
	return words, nil
}

// GenerateRecoveryKeyWithPassword unlocks SDEK using provided password and sets recovery wrapper.
func (m *Manager) GenerateRecoveryKeyWithPassword(password string, force bool) ([]string, error) {
	if password == "" {
		return nil, errors.New("password required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.inited {
		return nil, errors.New("not initialized")
	}
	b, err := os.ReadFile(m.path)
	if err != nil {
		return nil, err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st.SDEKRK != "" && !force {
		return nil, errors.New("recovery key already set")
	}
	salt, _ := base64.RawStdEncoding.DecodeString(st.Salt)
	key := m.deriveKey(password, salt, st.KDF)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	nonce, _ := base64.RawStdEncoding.DecodeString(st.Nonce)
	ct, _ := base64.RawStdEncoding.DecodeString(st.SDEK)
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, errors.New("invalid password")
	}
	// generate words and seal pt under RK
	words := make([]string, 24)
	rb := make([]byte, 24)
	if _, err := rand.Read(rb); err != nil {
		return nil, err
	}
	for i := 0; i < 24; i++ {
		words[i] = wordlist[int(rb[i])%len(wordlist)]
	}
	mnemonic := ""
	for i, w := range words {
		if i > 0 {
			mnemonic += " "
		}
		mnemonic += w
	}
	rkSalt := make([]byte, 16)
	_, _ = rand.Read(rkSalt)
	rkKey := m.deriveKey(mnemonic, rkSalt, st.KDF)
	block2, _ := aes.NewCipher(rkKey)
	aead2, _ := cipher.NewGCM(block2)
	rkNonce := make([]byte, aead2.NonceSize())
	_, _ = rand.Read(rkNonce)
	rkCT := aead2.Seal(nil, rkNonce, pt, nil)
	st.RKSalt = base64.RawStdEncoding.EncodeToString(rkSalt)
	st.RKNonce = base64.RawStdEncoding.EncodeToString(rkNonce)
	st.SDEKRK = base64.RawStdEncoding.EncodeToString(rkCT)
	nb, _ := json.MarshalIndent(&st, "", "  ")
	if err := os.WriteFile(m.path, nb, 0o600); err != nil {
		return nil, err
	}
	return words, nil
}

func (m *Manager) HasRecoveryKey() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, err := os.ReadFile(m.path)
	if err != nil {
		return false
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return false
	}
	return st.SDEKRK != ""
}

func (m *Manager) UnlockWithRecoveryKey(words []string) error {
	if len(words) == 0 {
		return errors.New("recovery_key required")
	}
	mn := ""
	for i, w := range words {
		if i > 0 {
			mn += " "
		}
		mn += w
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.inited {
		return errors.New("not initialized")
	}
	b, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.SDEKRK == "" {
		return errors.New("recovery key not set")
	}
	rkSalt, _ := base64.RawStdEncoding.DecodeString(st.RKSalt)
	rkKey := m.deriveKey(mn, rkSalt, st.KDF)
	block, _ := aes.NewCipher(rkKey)
	aead, _ := cipher.NewGCM(block)
	rkNonce, _ := base64.RawStdEncoding.DecodeString(st.RKNonce)
	rkCT, _ := base64.RawStdEncoding.DecodeString(st.SDEKRK)
	pt, err := aead.Open(nil, rkNonce, rkCT, nil)
	if err != nil {
		return errors.New("invalid recovery key")
	}
	m.sdek = pt
	return nil
}

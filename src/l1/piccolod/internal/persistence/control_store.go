package persistence

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"piccolod/internal/state/paths"
)

type keyProvider interface {
	WithSDEK(func([]byte) error) error
}

type encryptedControlStore struct {
	mu        sync.RWMutex
	filePath  string
	keySource keyProvider
	loaded    bool
	state     controlState
}

type controlState struct {
	authInitialized bool
	remoteConfig    *RemoteConfig
	apps            map[string]AppRecord
	passwordHash    string
	revision        uint64
	checksum        string
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

type encryptedPayload struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

const controlPayloadVersion = 1

const (
	controlVolumeMetadataName = "piccolo.volume.json"
	gocryptfsConfigName       = "gocryptfs.conf"
)

func newEncryptedControlStore(stateDir string, kp keyProvider) (*encryptedControlStore, error) {
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
	return &encryptedControlStore{
		filePath:  filepath.Join(cipherDir, "control.enc"),
		keySource: kp,
		state: controlState{
			apps: make(map[string]AppRecord),
		},
	}, nil
}

func (s *encryptedControlStore) Auth() AuthRepo         { return &authRepo{store: s} }
func (s *encryptedControlStore) Remote() RemoteRepo     { return &remoteRepo{store: s} }
func (s *encryptedControlStore) AppState() AppStateRepo { return &appStateRepo{store: s} }

func (s *encryptedControlStore) Close(ctx context.Context) error {
	_ = ctx
	s.Lock()
	return nil
}

func (s *encryptedControlStore) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded = false
	s.state = controlState{apps: make(map[string]AppRecord)}
}

func (s *encryptedControlStore) Revision(ctx context.Context) (uint64, string, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return 0, "", ErrLocked
	}
	return s.state.revision, s.state.checksum, nil
}

func (s *encryptedControlStore) Unlock(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	if s.keySource == nil {
		return ErrCryptoUnavailable
	}
	data := controlState{apps: make(map[string]AppRecord)}
	if _, err := os.Stat(s.filePath); err == nil {
		encBytes, err := os.ReadFile(s.filePath)
		if err != nil {
			return err
		}
		var enc encryptedPayload
		if err := json.Unmarshal(encBytes, &enc); err != nil {
			return err
		}
		var plaintext []byte
		if err := s.keySource.WithSDEK(func(key []byte) error {
			pt, derr := decryptControlPayload(key, enc)
			if derr != nil {
				return derr
			}
			plaintext = pt
			return nil
		}); err != nil {
			return err
		}
		var payload controlPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		data.authInitialized = payload.AuthInitialized
		data.passwordHash = payload.PasswordHash
		if payload.Remote != nil {
			rc := cloneRemoteConfig(*payload.Remote)
			data.remoteConfig = &rc
		}
		if len(payload.Apps) > 0 {
			for _, app := range payload.Apps {
				data.apps[app.Name] = app
			}
		}
		data.revision = payload.Revision
		data.checksum = payload.Checksum
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	s.state = data
	s.loaded = true
	return nil
}

func (s *encryptedControlStore) saveLocked() error {
	if !s.loaded {
		return ErrLocked
	}
	if err := s.ensureWritableLocked(); err != nil {
		return err
	}
	s.state.revision++
	payload := controlPayload{
		Version:         controlPayloadVersion,
		AuthInitialized: s.state.authInitialized,
		PasswordHash:    s.state.passwordHash,
		Revision:        s.state.revision,
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
	plainNoChecksum, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(plainNoChecksum))
	s.state.checksum = checksum
	payload.Checksum = checksum
	plain, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var serialized []byte
	err = s.keySource.WithSDEK(func(key []byte) error {
		enc, eerr := encryptControlPayload(key, plain)
		if eerr != nil {
			return eerr
		}
		serialized, eerr = json.Marshal(enc)
		return eerr
	})
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, serialized, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func cloneRemoteConfig(cfg RemoteConfig) RemoteConfig {
	dup := make([]byte, len(cfg.Payload))
	copy(dup, cfg.Payload)
	return RemoteConfig{Payload: dup}
}

// Repository implementations -------------------------------------------------

type authRepo struct{ store *encryptedControlStore }

type remoteRepo struct{ store *encryptedControlStore }

type appStateRepo struct{ store *encryptedControlStore }

func (r *authRepo) IsInitialized(ctx context.Context) (bool, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if !r.store.loaded {
		return false, ErrLocked
	}
	return r.store.state.authInitialized, nil
}

func (r *authRepo) SetInitialized(ctx context.Context) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	if r.store.state.authInitialized {
		return nil
	}
	r.store.state.authInitialized = true
	return r.store.saveLocked()
}

func (r *authRepo) PasswordHash(ctx context.Context) (string, error) {
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	if !r.store.loaded {
		return "", ErrLocked
	}
	return r.store.state.passwordHash, nil
}

func (r *authRepo) SavePasswordHash(ctx context.Context, hash string) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	r.store.state.passwordHash = hash
	return r.store.saveLocked()
}

func (r *remoteRepo) CurrentConfig(ctx context.Context) (RemoteConfig, error) {
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

func (r *remoteRepo) SaveConfig(ctx context.Context, cfg RemoteConfig) error {
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	copyCfg := cloneRemoteConfig(cfg)
	r.store.state.remoteConfig = &copyCfg
	return r.store.saveLocked()
}

func (r *appStateRepo) ListApps(ctx context.Context) ([]AppRecord, error) {
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

func (r *appStateRepo) UpsertApp(ctx context.Context, record AppRecord) error {
	if record.Name == "" {
		return errors.New("app name required")
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if err := r.store.ensureWritableLocked(); err != nil {
		return err
	}
	r.store.state.apps[record.Name] = record
	return r.store.saveLocked()
}

// Encryption helpers --------------------------------------------------------

func encryptControlPayload(key, plaintext []byte) (encryptedPayload, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedPayload{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedPayload{}, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return encryptedPayload{}, err
	}
	cipherText := aead.Seal(nil, nonce, plaintext, nil)
	return encryptedPayload{
		Version:    controlPayloadVersion,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(cipherText),
	}, nil
}

func decryptControlPayload(key []byte, payload encryptedPayload) ([]byte, error) {
	if payload.Version != controlPayloadVersion {
		return nil, errors.New("unsupported control payload version")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(payload.Nonce)
	if err != nil {
		return nil, err
	}
	cipherText, err := base64.RawStdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, cipherText, nil)
}

func (s *encryptedControlStore) ensureWritableLocked() error {
	if !s.loaded {
		return ErrLocked
	}
	return s.ensureCipherDirPreparedLocked()
}

// ensureCipherDirPreparedLocked verifies that the control volume has been
// initialised (metadata + gocryptfs config present) before we persist any
// payload. Callers must already hold s.mu.
func (s *encryptedControlStore) ensureCipherDirPreparedLocked() error {
	cipherDir := filepath.Dir(s.filePath)
	required := []string{
		filepath.Join(cipherDir, gocryptfsConfigName),
		filepath.Join(cipherDir, controlVolumeMetadataName),
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

var _ ControlStore = (*encryptedControlStore)(nil)

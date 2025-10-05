package persistence

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"piccolod/internal/crypt"
	"piccolod/internal/state/paths"
)

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string, stdin []byte) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// FileVolumeManager orchestrates gocryptfs-backed volumes rooted in PICCOLO_STATE_DIR.
type fileVolumeManager struct {
	root           string
	crypto         *crypt.Manager
	runner         commandRunner
	gocryptfsPath  string
	fusermountPath string
	volumes        map[string]*volumeEntry
	mu             sync.RWMutex
}

type volumeEntry struct {
	handle        VolumeHandle
	cipherDir     string
	metadata      volumeMetadata
	metadataReady bool
	role          VolumeRole
}

type volumeMetadata struct {
	Version    int    `json:"version"`
	WrappedKey string `json:"wrapped_key"`
	Nonce      string `json:"nonce"`
}

const (
	volumeMetadataName = "piccolo.volume.json"
	metadataVersion    = 1
)

func newFileVolumeManager(root string, crypto *crypt.Manager) *fileVolumeManager {
	if root == "" {
		root = paths.Root()
	}
	return &fileVolumeManager{
		root:           root,
		crypto:         crypto,
		runner:         execRunner{},
		gocryptfsPath:  defaultGocryptfsBinary(),
		fusermountPath: defaultFusermountBinary(),
		volumes:        make(map[string]*volumeEntry),
	}
}

// Helper for tests.
func newFileVolumeManagerWithDeps(root string, crypto *crypt.Manager, runner commandRunner, gocryptfsPath, fusermountPath string) *fileVolumeManager {
	mgr := newFileVolumeManager(root, crypto)
	if runner != nil {
		mgr.runner = runner
	}
	if gocryptfsPath != "" {
		mgr.gocryptfsPath = gocryptfsPath
	}
	if fusermountPath != "" {
		mgr.fusermountPath = fusermountPath
	}
	return mgr
}

func defaultGocryptfsBinary() string {
	if v := os.Getenv("PICCOLO_GOCRYPTFS_PATH"); v != "" {
		return v
	}
	return "gocryptfs"
}

func defaultFusermountBinary() string {
	if v := os.Getenv("PICCOLO_FUSERMOUNT_PATH"); v != "" {
		return v
	}
	if _, err := exec.LookPath("fusermount3"); err == nil {
		return "fusermount3"
	}
	if _, err := exec.LookPath("fusermount"); err == nil {
		return "fusermount"
	}
	return "fusermount3"
}

func (f *fileVolumeManager) EnsureVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if entry, ok := f.volumes[req.ID]; ok {
		return entry.handle, nil
	}

	cipherDir := filepath.Join(f.root, "ciphertext", req.ID)
	if err := os.MkdirAll(cipherDir, 0o700); err != nil {
		return VolumeHandle{}, fmt.Errorf("ensure volume %s ciphertext: %w", req.ID, err)
	}
	mountDir := filepath.Join(f.root, "mounts", req.ID)
	if err := os.MkdirAll(mountDir, 0o700); err != nil {
		return VolumeHandle{}, fmt.Errorf("ensure volume %s mount: %w", req.ID, err)
	}

	handle := VolumeHandle{ID: req.ID, MountDir: mountDir}
	entry := &volumeEntry{handle: handle, cipherDir: cipherDir}
	if err := f.ensureMetadata(ctx, entry); err != nil {
		if !errors.Is(err, crypt.ErrLocked) && !errors.Is(err, crypt.ErrNotInitialized) {
			return VolumeHandle{}, err
		}
	}

	f.volumes[req.ID] = entry
	return handle, nil
}

func (f *fileVolumeManager) Attach(ctx context.Context, handle VolumeHandle, opts AttachOptions) error {
	f.mu.RLock()
	entry, ok := f.volumes[handle.ID]
	f.mu.RUnlock()
	if !ok {
		return fmt.Errorf("attach: unknown volume %s", handle.ID)
	}

	if err := f.ensureMetadata(ctx, entry); err != nil {
		return err
	}

	passphrase, err := f.unwrapVolumeKey(ctx, entry.metadata)
	if err != nil {
		return err
	}

	args := []string{"-q", "-passfile", "/dev/stdin"}
	if opts.Role == VolumeRoleFollower {
		args = append(args, "-ro")
	}
	args = append(args, entry.cipherDir, entry.handle.MountDir)
	if err := f.runner.Run(ctx, f.gocryptfsPath, args, append(passphrase, '\n')); err != nil {
		return fmt.Errorf("mount volume %s: %w", handle.ID, err)
	}

	mode := []byte("rw")
	if opts.Role == VolumeRoleFollower {
		mode = []byte("ro")
	}
	if err := os.WriteFile(filepath.Join(entry.handle.MountDir, ".mode"), mode, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(entry.handle.MountDir, ".cipher"), []byte(entry.cipherDir), 0o600); err != nil {
		return err
	}

	f.mu.Lock()
	entry.role = opts.Role
	entry.metadataReady = true
	f.mu.Unlock()
	return nil
}

func (f *fileVolumeManager) Detach(ctx context.Context, handle VolumeHandle) error {
	args := []string{"-u", handle.MountDir}
	if err := f.runner.Run(ctx, f.fusermountPath, args, nil); err != nil {
		return fmt.Errorf("detach volume %s: %w", handle.ID, err)
	}
	return nil
}

func (f *fileVolumeManager) RoleStream(volumeID string) (<-chan VolumeRole, error) {
	ch := make(chan VolumeRole)
	close(ch)
	return ch, nil
}

func (f *fileVolumeManager) ensureMetadata(ctx context.Context, entry *volumeEntry) error {
	if entry.metadataReady {
		return nil
	}
	metaPath := filepath.Join(entry.cipherDir, volumeMetadataName)
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta volumeMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return err
		}
		entry.metadata = meta
		entry.metadataReady = true
		return nil
	}

	passphrase, err := generatePassphrase()
	if err != nil {
		return err
	}

	meta, err := f.sealVolumeKey(ctx, passphrase)
	if err != nil {
		return err
	}

	if err := f.runner.Run(ctx, f.gocryptfsPath, []string{"-q", "-init", "-passfile", "/dev/stdin", entry.cipherDir}, append(passphrase, '\n')); err != nil {
		return fmt.Errorf("init gocryptfs for %s: %w", entry.cipherDir, err)
	}

	metaBytes, err := json.MarshalIndent(&meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o600); err != nil {
		return err
	}

	entry.metadata = meta
	entry.metadataReady = true
	return nil
}

func (f *fileVolumeManager) sealVolumeKey(ctx context.Context, passphrase []byte) (volumeMetadata, error) {
	if f.crypto == nil {
		return volumeMetadata{}, errors.New("crypto manager unavailable")
	}
	meta := volumeMetadata{Version: metadataVersion}
	err := f.crypto.WithSDEK(func(sdek []byte) error {
		block, err := aes.NewCipher(sdek)
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
		sealed := aead.Seal(nil, nonce, passphrase, nil)
		meta.WrappedKey = base64.StdEncoding.EncodeToString(sealed)
		meta.Nonce = base64.StdEncoding.EncodeToString(nonce)
		return nil
	})
	if err != nil {
		return volumeMetadata{}, err
	}
	return meta, nil
}

func (f *fileVolumeManager) unwrapVolumeKey(ctx context.Context, meta volumeMetadata) ([]byte, error) {
	if f.crypto == nil {
		return nil, errors.New("crypto manager unavailable")
	}
	var passphrase []byte
	err := f.crypto.WithSDEK(func(sdek []byte) error {
		block, err := aes.NewCipher(sdek)
		if err != nil {
			return err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return err
		}
		nonce, err := base64.StdEncoding.DecodeString(meta.Nonce)
		if err != nil {
			return err
		}
		sealed, err := base64.StdEncoding.DecodeString(meta.WrappedKey)
		if err != nil {
			return err
		}
		key, err := aead.Open(nil, nonce, sealed, nil)
		if err != nil {
			return err
		}
		passphrase = key
		return nil
	})
	if err != nil {
		return nil, err
	}
	return passphrase, nil
}

func generatePassphrase() ([]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate passphrase: %w", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(raw)
	return []byte(encoded), nil
}

var _ VolumeManager = (*fileVolumeManager)(nil)

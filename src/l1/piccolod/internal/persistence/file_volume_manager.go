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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"piccolod/internal/crypt"
	"piccolod/internal/events"
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

func configureForegroundAttrs(cmd *exec.Cmd) {
	if runtime.GOOS != "linux" {
		return
	}
	attr := &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	cmd.SysProcAttr = attr
}

type mountProcess interface {
	Wait() <-chan error
	Signal(os.Signal) error
	Kill() error
	Pid() int
}

type mountLauncher interface {
	Launch(ctx context.Context, path string, args []string, stdin []byte) (mountProcess, error)
}

type mountWaiter func(mountPoint string, timeout time.Duration) error

type execMountLauncher struct{}

func (execMountLauncher) Launch(ctx context.Context, path string, args []string, stdin []byte) (mountProcess, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	configureForegroundAttrs(cmd)
	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		return nil, err
	}
	if stdin != nil {
		if _, err := io.Copy(stdinPipe, bytes.NewReader(stdin)); err != nil {
			stdinPipe.Close()
			_ = cmd.Process.Kill()
			return nil, err
		}
	}
	stdinPipe.Close()
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	return &execMountProcess{cmd: cmd, done: done}, nil
}

type execMountProcess struct {
	cmd  *exec.Cmd
	done chan error
}

func (p *execMountProcess) Wait() <-chan error { return p.done }

func (p *execMountProcess) Signal(sig os.Signal) error {
	if p.cmd.Process == nil {
		return errors.New("process not started")
	}
	return p.cmd.Process.Signal(sig)
}

func (p *execMountProcess) Kill() error {
	if p.cmd.Process == nil {
		return errors.New("process not started")
	}
	return p.cmd.Process.Kill()
}

func (p *execMountProcess) Pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// FileVolumeManager orchestrates gocryptfs-backed volumes rooted in PICCOLO_STATE_DIR.
type fileVolumeManager struct {
	root           string
	crypto         *crypt.Manager
	runner         commandRunner
	gocryptfsPath  string
	fusermountPath string
	volumes        map[string]*volumeEntry
	launcher       mountLauncher
	waitMount      mountWaiter
	stateRoot      string
	bus            *events.Bus
	roleChecker    func(string, VolumeRole) bool
	bypassMount    bool
	mu             sync.RWMutex
}

type volumeEntry struct {
	handle        VolumeHandle
	cipherDir     string
	metadata      volumeMetadata
	metadataReady bool
	role          VolumeRole
	process       mountProcess
	metaMu        sync.Mutex
}

type volumeMetadata struct {
	Version    int    `json:"version"`
	WrappedKey string `json:"wrapped_key"`
	Nonce      string `json:"nonce"`
}

type volumeState struct {
	Desired     string    `json:"desired_state"`
	Observed    string    `json:"observed_state"`
	Role        string    `json:"role"`
	LastError   string    `json:"last_error,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	Generation  int       `json:"generation"`
	NeedsRepair bool      `json:"needs_repair"`
	Description string    `json:"description,omitempty"`
}

const (
	volumeMetadataName   = "piccolo.volume.json"
	metadataVersion      = 1
	volumeStateMounted   = "mounted"
	volumeStateUnmounted = "unmounted"
	volumeStatePending   = "pending"
	volumeStateError     = "error"
)

var mountPointReplacer = strings.NewReplacer(
	`\040`, " ",
	`\011`, "\t",
	`\012`, "\n",
	`\134`, `\`,
)

func newFileVolumeManager(root string, crypto *crypt.Manager, bus *events.Bus) *fileVolumeManager {
	if root == "" {
		root = paths.Root()
	}
	bypass := os.Getenv("PICCOLO_ALLOW_UNMOUNTED_TESTS") == "1"
	waiter := waitForMountReady
	if bypass {
		waiter = func(string, time.Duration) error { return nil }
	}
	return &fileVolumeManager{
		root:           root,
		crypto:         crypto,
		runner:         execRunner{},
		gocryptfsPath:  defaultGocryptfsBinary(),
		fusermountPath: defaultFusermountBinary(),
		volumes:        make(map[string]*volumeEntry),
		launcher:       execMountLauncher{},
		waitMount:      waiter,
		stateRoot:      filepath.Join(root, "volumes"),
		bus:            bus,
		roleChecker:    func(string, VolumeRole) bool { return true },
		bypassMount:    bypass,
	}
}

// Helper for tests.
func newFileVolumeManagerWithDeps(root string, crypto *crypt.Manager, runner commandRunner, gocryptfsPath, fusermountPath string, launcher mountLauncher, waiter mountWaiter) *fileVolumeManager {
	mgr := newFileVolumeManager(root, crypto, nil)
	if runner != nil {
		mgr.runner = runner
	}
	if gocryptfsPath != "" {
		mgr.gocryptfsPath = gocryptfsPath
	}
	if fusermountPath != "" {
		mgr.fusermountPath = fusermountPath
	}
	if launcher != nil {
		mgr.launcher = launcher
	}
	if waiter != nil {
		mgr.waitMount = waiter
	}
	return mgr
}

func (f *fileVolumeManager) setRoleChecker(fn func(string, VolumeRole) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fn == nil {
		f.roleChecker = func(string, VolumeRole) bool { return true }
		return
	}
	f.roleChecker = fn
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

func (f *fileVolumeManager) getOrCreateEntry(id string) *volumeEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry, ok := f.volumes[id]; ok {
		return entry
	}
	entry := &volumeEntry{
		handle: VolumeHandle{
			ID:       id,
			MountDir: filepath.Join(f.root, "mounts", id),
		},
		cipherDir: filepath.Join(f.root, "ciphertext", id),
	}
	f.volumes[id] = entry
	return entry
}

func (f *fileVolumeManager) EnsureVolume(ctx context.Context, req VolumeRequest) (VolumeHandle, error) {
	f.mu.RLock()
	entry, ok := f.volumes[req.ID]
	f.mu.RUnlock()
	if ok {
		if err := f.reconcileVolumeState(ctx, entry); err != nil {
			return entry.handle, err
		}
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

	entry = &volumeEntry{
		handle:    VolumeHandle{ID: req.ID, MountDir: mountDir},
		cipherDir: cipherDir,
	}
	if err := f.ensureMetadata(ctx, entry); err != nil {
		if !errors.Is(err, crypt.ErrLocked) && !errors.Is(err, crypt.ErrNotInitialized) {
			return VolumeHandle{}, err
		}
	}

	f.mu.Lock()
	f.volumes[req.ID] = entry
	f.mu.Unlock()

	if err := f.reconcileVolumeState(ctx, entry); err != nil {
		return entry.handle, err
	}
	return entry.handle, nil
}

func (f *fileVolumeManager) Attach(ctx context.Context, handle VolumeHandle, opts AttachOptions) error {
	f.mu.RLock()
	entry, ok := f.volumes[handle.ID]
	checker := f.roleChecker
	f.mu.RUnlock()
	if !ok {
		return fmt.Errorf("attach: unknown volume %s", handle.ID)
	}
	if opts.Role == VolumeRoleLeader && checker != nil && !checker(handle.ID, opts.Role) {
		return fmt.Errorf("attach: leadership not granted for volume %s", handle.ID)
	}

	if f.bypassMount {
		if err := f.ensureMetadata(ctx, entry); err != nil {
			return err
		}
		modeBytes := []byte("rw")
		if opts.Role == VolumeRoleFollower {
			modeBytes = []byte("ro")
		}
		if err := os.WriteFile(filepath.Join(entry.handle.MountDir, ".mode"), modeBytes, 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(entry.handle.MountDir, ".cipher"), []byte(entry.cipherDir), 0o600); err != nil {
			return err
		}
		f.mu.Lock()
		entry.role = opts.Role
		entry.metadataReady = true
		f.mu.Unlock()
		return f.recordVolumeState(handle.ID, volumeStateMounted, volumeStateMounted, opts.Role, nil)
	}

	if err := f.ensureMetadata(ctx, entry); err != nil {
		return err
	}

	passphrase, err := f.unwrapVolumeKey(ctx, entry.metadata)
	if err != nil {
		return err
	}

	if err := f.recordVolumeState(handle.ID, volumeStateMounted, volumeStatePending, opts.Role, nil); err != nil {
		return err
	}

	args := []string{"-f", "-q", "-passfile", "/dev/stdin"}
	if opts.Role == VolumeRoleFollower {
		args = append(args, "-ro")
	}
	args = append(args, entry.cipherDir, entry.handle.MountDir)

	proc, err := f.launcher.Launch(ctx, f.gocryptfsPath, args, append(passphrase, '\n'))
	if err != nil {
		_ = f.recordVolumeState(handle.ID, volumeStateMounted, volumeStateError, opts.Role, err)
		return fmt.Errorf("mount volume %s: %w", handle.ID, err)
	}
	if err := f.waitMount(entry.handle.MountDir, 5*time.Second); err != nil {
		_ = proc.Signal(syscall.SIGTERM)
		select {
		case <-proc.Wait():
		case <-time.After(2 * time.Second):
			_ = proc.Kill()
			<-proc.Wait()
		}
		_ = f.recordVolumeState(handle.ID, volumeStateMounted, volumeStateError, opts.Role, err)
		return fmt.Errorf("wait for mount %s: %w", handle.ID, err)
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
	entry.process = proc
	f.mu.Unlock()
	if err := f.recordVolumeState(handle.ID, volumeStateMounted, volumeStateMounted, opts.Role, nil); err != nil {
		return err
	}
	return nil
}

func (f *fileVolumeManager) Detach(ctx context.Context, handle VolumeHandle) error {
	if f.bypassMount {
		role := VolumeRoleUnknown
		f.mu.RLock()
		if entry, ok := f.volumes[handle.ID]; ok {
			role = entry.role
			entry.role = VolumeRoleUnknown
		}
		f.mu.RUnlock()
		return f.recordVolumeState(handle.ID, volumeStateUnmounted, volumeStateUnmounted, role, nil)
	}
	role := VolumeRoleUnknown
	f.mu.RLock()
	if entry, ok := f.volumes[handle.ID]; ok {
		role = entry.role
	}
	f.mu.RUnlock()
	if err := f.recordVolumeState(handle.ID, volumeStateUnmounted, volumeStatePending, role, nil); err != nil {
		return err
	}
	args := []string{"-u", handle.MountDir}
	if err := f.runner.Run(ctx, f.fusermountPath, args, nil); err != nil {
		_ = f.recordVolumeState(handle.ID, volumeStateUnmounted, volumeStateError, role, err)
		return fmt.Errorf("detach volume %s: %w", handle.ID, err)
	}
	f.awaitProcessExit(handle.ID)
	if err := f.recordVolumeState(handle.ID, volumeStateUnmounted, volumeStateUnmounted, role, nil); err != nil {
		return err
	}
	return nil
}

func (f *fileVolumeManager) RoleStream(volumeID string) (<-chan VolumeRole, error) {
	ch := make(chan VolumeRole)
	close(ch)
	return ch, nil
}

func (f *fileVolumeManager) ensureMetadata(ctx context.Context, entry *volumeEntry) error {
	entry.metaMu.Lock()
	defer entry.metaMu.Unlock()

	metaPath := filepath.Join(entry.cipherDir, volumeMetadataName)
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta volumeMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return fmt.Errorf("%w: %v", ErrVolumeMetadataCorrupted, err)
		}
		if meta.WrappedKey == "" || meta.Nonce == "" {
			return fmt.Errorf("%w: missing fields", ErrVolumeMetadataCorrupted)
		}
		entry.metadata = meta
		entry.metadataReady = true
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read volume metadata %s: %w", metaPath, err)
	}

	entry.metadataReady = false
	if f.bypassMount {
		meta := volumeMetadata{
			Version:    metadataVersion,
			WrappedKey: base64.StdEncoding.EncodeToString([]byte("piccolo-bypass-key")),
			Nonce:      base64.StdEncoding.EncodeToString([]byte("bypass-nonce!!")),
		}
		metaBytes, err := json.MarshalIndent(&meta, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(metaPath, metaBytes, 0o600); err != nil {
			return err
		}
		confPath := filepath.Join(entry.cipherDir, gocryptfsConfigName)
		if _, err := os.Stat(confPath); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(confPath, []byte("bypass"), 0o600); err != nil {
				return err
			}
		} else if err != nil {
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

func (f *fileVolumeManager) recordVolumeState(volumeID string, desired string, observed string, role VolumeRole, cause error) error {
	if volumeID == "" {
		return nil
	}
	prev, err := f.readVolumeState(volumeID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	state := volumeState{
		Desired:   desired,
		Observed:  observed,
		Role:      string(role),
		UpdatedAt: time.Now().UTC(),
	}
	if cause != nil {
		state.LastError = cause.Error()
	} else if prev.LastError != "" && observed != volumeStateError {
		// Clear stale errors once we transition back to a stable state.
		state.LastError = ""
	}
	state.Generation = prev.Generation + 1
	state.NeedsRepair = prev.NeedsRepair
	if observed == volumeStateError || cause != nil {
		state.NeedsRepair = true
	}
	if (observed == volumeStateMounted || observed == volumeStateUnmounted) && cause == nil && observed == desired {
		state.NeedsRepair = false
	}
	if err := f.writeVolumeState(volumeID, state); err != nil {
		return err
	}
	f.publishVolumeEvent(volumeID, state)
	return nil
}

func (f *fileVolumeManager) readVolumeState(volumeID string) (volumeState, error) {
	path := filepath.Join(f.stateRoot, volumeID, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return volumeState{}, err
	}
	var state volumeState
	if err := json.Unmarshal(data, &state); err != nil {
		return volumeState{}, err
	}
	return state, nil
}

func (f *fileVolumeManager) publishVolumeEvent(volumeID string, state volumeState) {
	if f.bus == nil {
		return
	}
	f.bus.Publish(events.Event{
		Topic: events.TopicVolumeStateChanged,
		Payload: events.VolumeStateChanged{
			ID:          volumeID,
			Desired:     state.Desired,
			Observed:    state.Observed,
			Role:        state.Role,
			Generation:  state.Generation,
			NeedsRepair: state.NeedsRepair,
			LastError:   state.LastError,
		},
	})
}

func (f *fileVolumeManager) reconcileVolumeState(ctx context.Context, entry *volumeEntry) error {
	if f.bypassMount {
		return nil
	}
	state, err := f.readVolumeState(entry.handle.ID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	mounted, err := isMountPoint(entry.handle.MountDir)
	if err != nil {
		return err
	}
	role := parseVolumeRole(state.Role)
	if state.Desired == volumeStateMounted && !mounted {
		checker := f.roleChecker
		if state.Role == string(VolumeRoleLeader) && checker != nil && !checker(entry.handle.ID, VolumeRoleLeader) {
			return nil
		}
		if role == VolumeRoleUnknown {
			return f.recordVolumeState(entry.handle.ID, state.Desired, volumeStateError, role, fmt.Errorf("volume %s missing mount and role unknown", entry.handle.ID))
		}
		if err := f.Attach(ctx, entry.handle, AttachOptions{Role: role}); err != nil {
			if errors.Is(err, crypt.ErrLocked) || errors.Is(err, crypt.ErrNotInitialized) {
				if recErr := f.recordVolumeState(entry.handle.ID, state.Desired, volumeStatePending, role, err); recErr != nil {
					return recErr
				}
				return nil
			}
			return err
		}
		return nil
	}
	if state.Desired == volumeStateUnmounted && mounted {
		if err := f.Detach(ctx, entry.handle); err != nil {
			return err
		}
		return nil
	}
	if state.NeedsRepair && state.Desired == volumeStateMounted && mounted {
		return f.recordVolumeState(entry.handle.ID, state.Desired, volumeStateMounted, role, nil)
	}
	if state.NeedsRepair && state.Desired == volumeStateUnmounted && !mounted {
		return f.recordVolumeState(entry.handle.ID, state.Desired, volumeStateUnmounted, role, nil)
	}
	return nil
}

func parseVolumeRole(role string) VolumeRole {
	switch role {
	case string(VolumeRoleLeader):
		return VolumeRoleLeader
	case string(VolumeRoleFollower):
		return VolumeRoleFollower
	default:
		return VolumeRoleUnknown
	}
}

func (f *fileVolumeManager) reconcileAllVolumeStates() error {
	entries, err := os.ReadDir(f.stateRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		volID := entry.Name()
		volEntry := f.getOrCreateEntry(volID)
		if err := f.reconcileVolumeState(context.Background(), volEntry); err != nil {
			return err
		}
	}
	return nil
}

func (f *fileVolumeManager) writeVolumeState(volumeID string, state volumeState) error {
	dir := filepath.Join(f.stateRoot, volumeID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&state); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	finalPath := filepath.Join(dir, "state.json")
	if err := os.Rename(tmp.Name(), finalPath); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return syncDir(f.stateRoot)
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
			return fmt.Errorf("%w: decode nonce: %v", ErrVolumeMetadataCorrupted, err)
		}
		if len(nonce) != aead.NonceSize() {
			return fmt.Errorf("%w: invalid nonce length %d (expected %d)", ErrVolumeMetadataCorrupted, len(nonce), aead.NonceSize())
		}
		sealed, err := base64.StdEncoding.DecodeString(meta.WrappedKey)
		if err != nil {
			return fmt.Errorf("%w: decode wrapped key: %v", ErrVolumeMetadataCorrupted, err)
		}
		if len(sealed) == 0 {
			return fmt.Errorf("%w: empty wrapped key", ErrVolumeMetadataCorrupted)
		}
		key, err := aead.Open(nil, nonce, sealed, nil)
		if err != nil {
			return fmt.Errorf("%w: unwrap failed: %v", ErrVolumeMetadataCorrupted, err)
		}
		passphrase = key
		return nil
	})
	if err != nil {
		return nil, err
	}
	return passphrase, nil
}

func (f *fileVolumeManager) awaitProcessExit(volumeID string) {
	f.mu.Lock()
	entry, ok := f.volumes[volumeID]
	if !ok {
		f.mu.Unlock()
		return
	}
	proc := entry.process
	entry.process = nil
	f.mu.Unlock()
	if proc == nil {
		return
	}
	select {
	case <-proc.Wait():
	case <-time.After(2 * time.Second):
		_ = proc.Kill()
		<-proc.Wait()
	}
}

func waitForMountReady(mountPoint string, timeout time.Duration) error {
	mountPoint = filepath.Clean(mountPoint)
	deadline := time.Now().Add(timeout)
	for {
		mounted, err := isMountPoint(mountPoint)
		if err != nil {
			return err
		}
		if mounted {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for mount %s", mountPoint)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func isMountPoint(mountPoint string) (bool, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	target := filepath.Clean(mountPoint)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, " ")
		if len(fields) < 5 {
			continue
		}
		pathField := decodeMountPoint(fields[4])
		if pathField == target {
			return true, nil
		}
	}
	return false, nil
}

func decodeMountPoint(raw string) string {
	return mountPointReplacer.Replace(raw)
}

func generatePassphrase() ([]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate passphrase: %w", err)
	}
	encoded := base64.RawStdEncoding.EncodeToString(raw)
	return []byte(encoded), nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

var _ VolumeManager = (*fileVolumeManager)(nil)

package persistence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"piccolod/internal/crypt"
	"piccolod/internal/events"
)

type runnerCall struct {
	name  string
	args  []string
	stdin string
}

type fakeRunner struct {
	calls []runnerCall
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, stdin []byte) error {
	call := runnerCall{name: name, args: append([]string(nil), args...), stdin: string(stdin)}
	f.calls = append(f.calls, call)
	return nil
}

type fakeMountLauncher struct {
	calls     []runnerCall
	processes []*fakeMountProcess
}

func (f *fakeMountLauncher) Launch(ctx context.Context, name string, args []string, stdin []byte) (mountProcess, error) {
	call := runnerCall{name: name, args: append([]string(nil), args...), stdin: string(stdin)}
	f.calls = append(f.calls, call)
	proc := &fakeMountProcess{done: make(chan error, 1)}
	f.processes = append(f.processes, proc)
	return proc, nil
}

type fakeMountProcess struct {
	done chan error
}

func (p *fakeMountProcess) Wait() <-chan error {
	return p.done
}

func (p *fakeMountProcess) Signal(os.Signal) error { return nil }

func (p *fakeMountProcess) Kill() error {
	select {
	case p.done <- errors.New("killed"):
	default:
	}
	return nil
}

func (p *fakeMountProcess) Pid() int { return 1234 }

type fakeTimeoutProcess struct {
	waitCh   chan error
	signaled bool
	killed   bool
}

func newFakeTimeoutProcess() *fakeTimeoutProcess {
	return &fakeTimeoutProcess{waitCh: make(chan error, 1)}
}

func (p *fakeTimeoutProcess) Wait() <-chan error {
	return p.waitCh
}

func (p *fakeTimeoutProcess) Signal(os.Signal) error {
	p.signaled = true
	select {
	case p.waitCh <- errors.New("terminated"):
	default:
	}
	return nil
}

func (p *fakeTimeoutProcess) Kill() error {
	p.killed = true
	select {
	case p.waitCh <- errors.New("killed"):
	default:
	}
	return nil
}

func (p *fakeTimeoutProcess) Pid() int {
	return 1234
}

type timeoutMountLauncher struct {
	calls   []runnerCall
	process *fakeTimeoutProcess
}

func (t *timeoutMountLauncher) Launch(ctx context.Context, name string, args []string, stdin []byte) (mountProcess, error) {
	call := runnerCall{name: name, args: append([]string(nil), args...), stdin: string(stdin)}
	t.calls = append(t.calls, call)
	return t.process, nil
}

func newUnlockedCrypto(t *testing.T, dir string) *crypt.Manager {
	mgr, err := crypt.NewManager(dir)
	if err != nil {
		t.Fatalf("new crypto manager: %v", err)
	}
	if err := mgr.Setup("passphrase"); err != nil && !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("crypto setup: %v", err)
	}
	if err := mgr.Unlock("passphrase"); err != nil {
		t.Fatalf("crypto unlock: %v", err)
	}
	return mgr
}

func TestFileVolumeManagerEnsureVolume(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", nil, nil)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "control", Class: VolumeClassControl})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	expectedMount := filepath.Join(root, "mounts", "control")
	if handle.MountDir != expectedMount {
		t.Fatalf("expected mount dir %s, got %s", expectedMount, handle.MountDir)
	}
	if _, err := os.Stat(expectedMount); err != nil {
		t.Fatalf("mount dir missing: %v", err)
	}
	cipherDir := filepath.Join(root, "ciphertext", "control")
	if _, err := os.Stat(cipherDir); err != nil {
		t.Fatalf("cipher dir missing: %v", err)
	}
	metaPath := filepath.Join(cipherDir, volumeMetadataName)
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("metadata missing: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "gocryptfs" || !containsArgs(call.args, []string{"-q", "-init", "-passfile", "/dev/stdin"}) {
		t.Fatalf("unexpected init call: %+v", call)
	}
	if !strings.HasSuffix(call.stdin, "\n") {
		t.Fatalf("expected newline-terminated passphrase, got %q", call.stdin)
	}
	passphrase := strings.TrimSpace(call.stdin)
	if _, err := base64.RawStdEncoding.DecodeString(passphrase); err != nil {
		t.Fatalf("expected base64 passphrase, decode error: %v", err)
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var meta volumeMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.WrappedKey == "" || meta.Nonce == "" {
		t.Fatalf("metadata missing wrapped key or nonce: %+v", meta)
	}

	// Repeated ensure should not re-run init
	handle2, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "control", Class: VolumeClassControl})
	if err != nil {
		t.Fatalf("EnsureVolume second: %v", err)
	}
	if handle2.MountDir != handle.MountDir {
		t.Fatalf("expected same mount dir, got %s vs %s", handle2.MountDir, handle.MountDir)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected no additional commands, got %d", len(runner.calls))
	}
}

func TestFileVolumeManagerAttachRoles(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, func(string, time.Duration) error { return nil })

	h, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "alpha", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	launcher.calls = launcher.calls[:0]

	if err := mgr.Attach(context.Background(), h, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("attach leader: %v", err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("expected mount call, got %d", len(launcher.calls))
	}
	if data, err := os.ReadFile(filepath.Join(h.MountDir, ".mode")); err != nil || string(data) != "rw" {
		t.Fatalf("expected mode rw, got %v %q", err, string(data))
	}

	if !containsArgs(launcher.calls[0].args, []string{"-f", "-q", "-passfile", "/dev/stdin"}) {
		t.Fatalf("unexpected leader args: %+v", launcher.calls[0].args)
	}

	launcher.calls = launcher.calls[:0]
	if err := mgr.Attach(context.Background(), h, AttachOptions{Role: VolumeRoleFollower}); err != nil {
		t.Fatalf("attach follower: %v", err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("expected mount call, got %d", len(launcher.calls))
	}
	call := launcher.calls[0]
	if !containsArgs(call.args, []string{"-ro"}) {
		t.Fatalf("expected -ro in follower args, got %+v", call.args)
	}
	if data, err := os.ReadFile(filepath.Join(h.MountDir, ".mode")); err != nil || string(data) != "ro" {
		t.Fatalf("expected mode ro, got %v %q", err, string(data))
	}
}

func TestFileVolumeManagerDetach(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", nil, nil)

	h, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "beta", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	runner.calls = runner.calls[:0]

	if err := mgr.Detach(context.Background(), h); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected fusermount call, got %d", len(runner.calls))
	}
	if runner.calls[0].name != "fusermount3" {
		t.Fatalf("expected fusermount3, got %s", runner.calls[0].name)
	}
	if !containsArgs(runner.calls[0].args, []string{"-u", h.MountDir}) {
		t.Fatalf("unexpected fusermount args: %+v", runner.calls[0].args)
	}
}

func TestFileVolumeManagerAttachDetectsCorruptedMetadata(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, func(string, time.Duration) error { return nil })

	if _, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "corrupt", Class: VolumeClassApplication}); err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	metaPath := filepath.Join(root, "ciphertext", "corrupt", volumeMetadataName)
	if err := os.WriteFile(metaPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("corrupt metadata: %v", err)
	}

	// Simulate manager restart; cached metadata should not mask corruption.
	runner2 := &fakeRunner{}
	launcher2 := &fakeMountLauncher{}
	mgr2 := newFileVolumeManagerWithDeps(root, cryptoMgr, runner2, "gocryptfs", "fusermount3", launcher2, func(string, time.Duration) error { return nil })

	_, err := mgr2.EnsureVolume(context.Background(), VolumeRequest{ID: "corrupt", Class: VolumeClassApplication})
	if err == nil {
		t.Fatalf("expected EnsureVolume to fail due to corrupted metadata")
	}
	if !errors.Is(err, ErrVolumeMetadataCorrupted) {
		t.Fatalf("expected ErrVolumeMetadataCorrupted, got %v", err)
	}
	if len(runner2.calls) != 0 {
		t.Fatalf("expected failure before invoking gocryptfs init, got %d calls", len(runner2.calls))
	}
	if len(launcher2.calls) != 0 {
		t.Fatalf("expected failure before launching gocryptfs, got %d calls", len(launcher2.calls))
	}
}

func TestFileVolumeManagerAttachFailsWhenMetadataCorruptedWhileRunning(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	waiter := func(string, time.Duration) error { return nil }
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, waiter)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "livecorrupt", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	metaPath := filepath.Join(root, "ciphertext", "livecorrupt", volumeMetadataName)
	if err := os.WriteFile(metaPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("corrupt metadata: %v", err)
	}

	err = mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader})
	if err == nil {
		t.Fatalf("expected Attach to fail due to corrupted metadata")
	}
	if !errors.Is(err, ErrVolumeMetadataCorrupted) {
		t.Fatalf("expected ErrVolumeMetadataCorrupted, got %v", err)
	}
	if len(launcher.calls) != 0 {
		t.Fatalf("expected metadata failure before launching gocryptfs, got %d calls", len(launcher.calls))
	}
}

func TestFileVolumeManagerAttachFailsWithInvalidMetadataValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*volumeMetadata)
	}{
		{
			name: "invalid nonce encoding",
			mutate: func(m *volumeMetadata) {
				m.Nonce = "%%%not-base64%%%"
			},
		},
		{
			name: "invalid nonce length",
			mutate: func(m *volumeMetadata) {
				m.Nonce = base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
			},
		},
		{
			name: "invalid wrapped key encoding",
			mutate: func(m *volumeMetadata) {
				m.WrappedKey = "!!!"
			},
		},
		{
			name: "tampered wrapped key",
			mutate: func(m *volumeMetadata) {
				sealed, err := base64.StdEncoding.DecodeString(m.WrappedKey)
				if err != nil || len(sealed) == 0 {
					m.WrappedKey = "invalid"
					return
				}
				sealed[0] ^= 0xFF
				m.WrappedKey = base64.StdEncoding.EncodeToString(sealed)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cryptoMgr := newUnlockedCrypto(t, root)
			runner := &fakeRunner{}
			launcher := &fakeMountLauncher{}
			waiter := func(string, time.Duration) error { return nil }
			mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, waiter)

			handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "victim", Class: VolumeClassApplication})
			if err != nil {
				t.Fatalf("EnsureVolume: %v", err)
			}

			metaPath := filepath.Join(root, "ciphertext", "victim", volumeMetadataName)
			metaBytes, err := os.ReadFile(metaPath)
			if err != nil {
				t.Fatalf("read metadata: %v", err)
			}

			var meta volumeMetadata
			if err := json.Unmarshal(metaBytes, &meta); err != nil {
				t.Fatalf("unmarshal metadata: %v", err)
			}
			tc.mutate(&meta)
			updated, err := json.MarshalIndent(&meta, "", "  ")
			if err != nil {
				t.Fatalf("marshal metadata: %v", err)
			}
			if err := os.WriteFile(metaPath, updated, 0o600); err != nil {
				t.Fatalf("write metadata: %v", err)
			}

			launcher.calls = nil
			err = mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader})
			if err == nil {
				t.Fatalf("expected attach to fail due to metadata issue")
			}
			if !errors.Is(err, ErrVolumeMetadataCorrupted) {
				t.Fatalf("expected ErrVolumeMetadataCorrupted, got %v", err)
			}
			if len(launcher.calls) != 0 {
				t.Fatalf("expected metadata failure before launching gocryptfs, got %d calls", len(launcher.calls))
			}
		})
	}
}

func TestFileVolumeManagerAttachHandlesMountTimeout(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	timeoutProc := newFakeTimeoutProcess()
	launcher := &timeoutMountLauncher{process: timeoutProc}
	waiter := func(string, time.Duration) error {
		return errors.New("mount timed out")
	}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, waiter)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "timeout", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	err = mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader})
	if err == nil {
		t.Fatalf("expected mount timeout error")
	}
	if !strings.Contains(err.Error(), "wait for mount timeout") && !strings.Contains(err.Error(), "mount timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if !timeoutProc.signaled {
		t.Fatalf("expected mount process to receive SIGTERM on timeout")
	}
	if timeoutProc.killed {
		t.Fatalf("expected SIGTERM to unblock wait without Kill")
	}
}

func TestFileVolumeManagerRecordsMountedState(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	waiter := func(string, time.Duration) error { return nil }
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, waiter)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "journal", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if err := mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach leader: %v", err)
	}

	statePath := filepath.Join(root, "volumes", handle.ID, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("expected state journal at %s: %v", statePath, err)
	}

	var state struct {
		Desired     string `json:"desired_state"`
		Observed    string `json:"observed_state"`
		Role        string `json:"role"`
		Generation  int    `json:"generation"`
		NeedsRepair bool   `json:"needs_repair"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal journal: %v", err)
	}
	if state.Desired != "mounted" || state.Observed != "mounted" || state.Role != string(VolumeRoleLeader) {
		t.Fatalf("unexpected journal state: %+v", state)
	}
	if state.Generation <= 0 {
		t.Fatalf("expected generation > 0, got %+v", state)
	}
	if state.NeedsRepair {
		t.Fatalf("expected NeedsRepair false after successful mount: %+v", state)
	}
}

func TestFileVolumeManagerPublishesVolumeEvents(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	bus := events.NewBus()
	sub := bus.Subscribe(events.TopicVolumeStateChanged, 10)

	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, func(string, time.Duration) error { return nil })
	mgr.bus = bus

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "eventful", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if err := mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var mounted events.VolumeStateChanged
	for mounted.Observed != "mounted" {
		select {
		case evt := <-sub:
			state, ok := evt.Payload.(events.VolumeStateChanged)
			if !ok || state.ID != "eventful" {
				continue
			}
			mounted = state
		case <-ctx.Done():
			t.Fatalf("timeout waiting for mounted event")
		}
	}
	if mounted.Generation <= 0 || mounted.NeedsRepair || mounted.Desired != "mounted" || mounted.Role != string(VolumeRoleLeader) {
		t.Fatalf("unexpected mounted event payload: %+v", mounted)
	}

	if err := mgr.Detach(context.Background(), handle); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	var unmounted events.VolumeStateChanged
	for unmounted.Observed != "unmounted" {
		select {
		case evt := <-sub:
			state, ok := evt.Payload.(events.VolumeStateChanged)
			if !ok || state.ID != "eventful" {
				continue
			}
			unmounted = state
		case <-ctx2.Done():
			t.Fatalf("timeout waiting for unmounted event")
		}
	}
	if unmounted.Desired != "unmounted" || unmounted.NeedsRepair || unmounted.LastError != "" {
		t.Fatalf("unexpected unmounted event payload: %+v", unmounted)
	}
}

func TestFileVolumeManagerReconcilesStaleMountedStateOnEnsure(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	waiter := func(string, time.Duration) error { return nil }
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, waiter)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "stale", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if err := mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach leader: %v", err)
	}

	// Simulate crash: drop mount markers so the mount is definitely absent.
	if err := os.Remove(filepath.Join(handle.MountDir, ".mode")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove mode marker: %v", err)
	}
	if err := os.Remove(filepath.Join(handle.MountDir, ".cipher")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove cipher marker: %v", err)
	}

	statePath := filepath.Join(root, "volumes", handle.ID, "state.json")
	initialStateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read initial state: %v", err)
	}
	var initialState volumeState
	if err := json.Unmarshal(initialStateBytes, &initialState); err != nil {
		t.Fatalf("unmarshal initial state: %v", err)
	}
	if initialState.Observed != "mounted" {
		t.Fatalf("expected initial observed mounted, got %+v", initialState)
	}

	// Restart manager.
	runner2 := &fakeRunner{}
	launcher2 := &fakeMountLauncher{}
	mgr2 := newFileVolumeManagerWithDeps(root, cryptoMgr, runner2, "gocryptfs", "fusermount3", launcher2, waiter)

	if err := mgr2.reconcileAllVolumeStates(); err != nil {
		t.Fatalf("reconcileAllVolumeStates: %v", err)
	}

	if len(launcher2.calls) == 0 {
		t.Fatalf("expected auto-reattach during startup")
	}

	if _, err := mgr2.EnsureVolume(context.Background(), VolumeRequest{ID: "stale", Class: VolumeClassApplication}); err != nil {
		t.Fatalf("EnsureVolume after crash: %v", err)
	}

	updatedBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read reconciled state: %v", err)
	}
	var updated volumeState
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("unmarshal reconciled state: %v", err)
	}
	if updated.Observed != "mounted" {
		t.Fatalf("expected observed=mounted after auto-reattach, got %+v", updated)
	}
	if updated.NeedsRepair {
		t.Fatalf("expected NeedsRepair false after successful reattach %+v", updated)
	}
	if updated.LastError != "" {
		t.Fatalf("expected LastError cleared, got %+v", updated.LastError)
	}
}

func TestFileVolumeManagerReconcileSkipsLeaderWithoutAuthority(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	launcher := &fakeMountLauncher{}
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, &fakeRunner{}, "gocryptfs", "fusermount3", launcher, func(string, time.Duration) error { return nil })
	mgr.setRoleChecker(func(string, VolumeRole) bool { return false })

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "authority", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	if err := mgr.recordVolumeState(handle.ID, volumeStateMounted, volumeStateMounted, VolumeRoleLeader, nil); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	if err := mgr.reconcileAllVolumeStates(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := len(launcher.calls); got != 0 {
		t.Fatalf("expected no attach when authority missing, got %d", got)
	}

	mgr.setRoleChecker(func(string, VolumeRole) bool { return true })
	if err := mgr.reconcileAllVolumeStates(); err != nil {
		t.Fatalf("reconcile with authority: %v", err)
	}
	if got := len(launcher.calls); got != 1 {
		t.Fatalf("expected one attach when authority restored, got %d", got)
	}
}

func TestFileVolumeManagerReconcileClearsNeedsRepairOnUnmounted(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, &fakeRunner{}, "gocryptfs", "fusermount3", nil, func(string, time.Duration) error { return nil })

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "cleanup", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	if err := mgr.recordVolumeState(handle.ID, volumeStateUnmounted, volumeStateError, VolumeRoleFollower, errors.New("detach failed")); err != nil {
		t.Fatalf("seed error state: %v", err)
	}

	if err := mgr.reconcileAllVolumeStates(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	state, err := mgr.readVolumeState(handle.ID)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.NeedsRepair {
		t.Fatalf("expected NeedsRepair cleared, got %+v", state)
	}
	if state.Observed != volumeStateUnmounted || state.LastError != "" {
		t.Fatalf("unexpected reconciled state: %+v", state)
	}
}

func TestFileVolumeManagerReconcileHandlesLockedCrypto(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	waiter := func(string, time.Duration) error { return nil }
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", nil, waiter)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "bootstrap", Class: VolumeClassBootstrap})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if err := mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach leader: %v", err)
	}

	cryptoMgr.Lock()
	lockedMgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", nil, waiter)
	if err := lockedMgr.reconcileAllVolumeStates(); err != nil {
		t.Fatalf("reconcileAllVolumeStates: %v", err)
	}

	statePath := filepath.Join(root, "volumes", handle.ID, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state volumeState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if state.Observed != volumeStatePending {
		t.Fatalf("expected observed state pending, got %s", state.Observed)
	}
	if !strings.Contains(state.LastError, "locked") {
		t.Fatalf("expected last error to mention locked, got %q", state.LastError)
	}
}

func TestFileVolumeManagerStateGenerationAndNeedsRepair(t *testing.T) {
	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	runner := &fakeRunner{}
	launcher := &fakeMountLauncher{}
	waiter := func(string, time.Duration) error { return nil }
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, runner, "gocryptfs", "fusermount3", launcher, waiter)

	handle, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "gen", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	if err := mgr.Attach(context.Background(), handle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach leader: %v", err)
	}

	statePath := filepath.Join(root, "volumes", handle.ID, "state.json")
	initialBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var initial volumeState
	if err := json.Unmarshal(initialBytes, &initial); err != nil {
		t.Fatalf("unmarshal initial state: %v", err)
	}
	if initial.Generation == 0 {
		t.Fatalf("expected generation to be > 0, got %+v", initial)
	}
	if initial.NeedsRepair {
		t.Fatalf("expected needsRepair false after successful mount")
	}

	// Simulate crash resulting in missing mount.
	if err := os.Remove(filepath.Join(handle.MountDir, ".mode")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove .mode: %v", err)
	}
	if err := os.Remove(filepath.Join(handle.MountDir, ".cipher")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove .cipher: %v", err)
	}

	launcherFail := &fakeMountLauncher{}
	waiterFail := func(string, time.Duration) error {
		return errors.New("mount timed out")
	}
	mgr2 := newFileVolumeManagerWithDeps(root, cryptoMgr, &fakeRunner{}, "gocryptfs", "fusermount3", launcherFail, waiterFail)
	_ = mgr2.reconcileAllVolumeStates()
	if _, err := mgr2.EnsureVolume(context.Background(), VolumeRequest{ID: "gen", Class: VolumeClassApplication}); err == nil {
		t.Fatalf("expected EnsureVolume to surface auto-reattach failure")
	}

	updatedBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read updated state: %v", err)
	}
	var updated volumeState
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("unmarshal updated state: %v", err)
	}
	if updated.Generation <= initial.Generation {
		t.Fatalf("expected generation to increment %+v -> %+v", initial, updated)
	}
	if !updated.NeedsRepair {
		t.Fatalf("expected needsRepair true after reconciliation %+v", updated)
	}
}

func containsArgs(args []string, target []string) bool {
	for _, t := range target {
		found := false
		for _, a := range args {
			if a == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestFileVolumeManagerIntegration(t *testing.T) {
	if os.Getenv("PICCOLO_TEST_GOCRYPTFS") == "" {
		t.Skip("set PICCOLO_TEST_GOCRYPTFS=1 to run gocryptfs integration test")
	}
	if _, err := exec.LookPath("gocryptfs"); err != nil {
		t.Skip("gocryptfs binary not found")
	}
	fusermount := "fusermount3"
	if _, err := exec.LookPath(fusermount); err != nil {
		if _, err := exec.LookPath("fusermount"); err == nil {
			fusermount = "fusermount"
		} else {
			t.Skip("fusermount binary not found")
		}
	}
	if f, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0); err != nil {
		t.Skipf("fuse device unavailable: %v", err)
	} else {
		_ = f.Close()
	}

	root := t.TempDir()
	cryptoMgr := newUnlockedCrypto(t, root)
	mgr := newFileVolumeManagerWithDeps(root, cryptoMgr, execRunner{}, "gocryptfs", fusermount, nil, nil)

	h, err := mgr.EnsureVolume(context.Background(), VolumeRequest{ID: "integration", Class: VolumeClassApplication})
	if err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	mounted := false
	t.Cleanup(func() {
		if mounted {
			_ = mgr.Detach(context.Background(), h)
		}
	})

	if err := mgr.Attach(ctx, h, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		t.Fatalf("Attach leader: %v", err)
	}
	mounted = true

	message := []byte("hello from gocryptfs integration test")
	if err := os.WriteFile(filepath.Join(h.MountDir, "test.txt"), message, 0o600); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}

	// Ensure the ciphertext directory does not contain the plaintext string.
	cipherData, err := os.ReadFile(filepath.Join(root, "ciphertext", "integration", "gocryptfs.conf"))
	if err != nil {
		t.Fatalf("read ciphertext metadata: %v", err)
	}
	if strings.Contains(string(cipherData), string(message)) {
		t.Fatalf("ciphertext unexpectedly contains plaintext")
	}

	if err := mgr.Detach(ctx, h); err != nil {
		t.Fatalf("Detach: %v", err)
	}
}

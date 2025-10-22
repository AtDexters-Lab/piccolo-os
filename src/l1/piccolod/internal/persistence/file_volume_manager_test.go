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

func (p *fakeMountProcess) Wait() <-chan error { return p.done }

func (p *fakeMountProcess) Signal(os.Signal) error { return nil }

func (p *fakeMountProcess) Kill() error {
	select {
	case p.done <- errors.New("killed"):
	default:
	}
	return nil
}

func (p *fakeMountProcess) Pid() int { return 1234 }

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

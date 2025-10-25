package persistence

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"piccolod/internal/cluster"
	"piccolod/internal/crypt"
	"piccolod/internal/state/paths"
)

func TestPersistenceStateDirHasNoPlaintextArtifacts(t *testing.T) {
	gocryptfsPath, err := exec.LookPath("gocryptfs")
	if err != nil {
		t.Fatalf("gocryptfs binary required: %v", err)
	}

	fusermountPath := "fusermount3"
	if _, err := exec.LookPath(fusermountPath); err != nil {
		if _, err := exec.LookPath("fusermount"); err == nil {
			fusermountPath = "fusermount"
		} else {
			t.Fatalf("fusermount binary required: %v", err)
		}
	}

	if f, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0); err != nil {
		t.Fatalf("FUSE device required: %v", err)
	} else {
		f.Close()
	}

	stateDir := t.TempDir()
	t.Setenv("PICCOLO_STATE_DIR", stateDir)
	t.Setenv("PICCOLO_GOCRYPTFS_PATH", gocryptfsPath)
	t.Setenv("PICCOLO_FUSERMOUNT_PATH", fusermountPath)
	paths.SetRootForTest(stateDir)

	password := "audit-passphrase"
	cryptoMgr, err := crypt.NewManager(stateDir)
	if err != nil {
		t.Fatalf("crypto manager init: %v", err)
	}
	if !cryptoMgr.IsInitialized() {
		if err := cryptoMgr.Setup(password); err != nil {
			t.Fatalf("crypto setup: %v", err)
		}
	}
	if err := cryptoMgr.Unlock(password); err != nil {
		t.Fatalf("crypto unlock: %v", err)
	}

	mod, err := NewService(Options{
		Crypto:   cryptoMgr,
		StateDir: stateDir,
	})
	if err != nil {
		t.Fatalf("persistence service init: %v", err)
	}
	t.Cleanup(func() {
		_ = mod.Shutdown(context.Background())
	})

	ctx := context.Background()
	controlHandle, err := mod.Volumes().EnsureVolume(ctx, VolumeRequest{
		ID:          "control",
		Class:       VolumeClassControl,
		ClusterMode: ClusterModeStateful,
	})
	if err != nil {
		t.Fatalf("ensure control volume: %v", err)
	}

	if err := mod.Volumes().Attach(ctx, controlHandle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
		mod.leadership.Set(cluster.ResourceKernel, cluster.RoleLeader)
		if err := mod.Volumes().Attach(ctx, controlHandle, AttachOptions{Role: VolumeRoleLeader}); err != nil {
			t.Fatalf("attach control volume: %v", err)
		}
	}

	plaintextPath := filepath.Join(controlHandle.MountDir, "audit.txt")
	if err := os.WriteFile(plaintextPath, []byte("plaintext sentinel data"), 0o600); err != nil {
		t.Fatalf("write plaintext to control mount: %v", err)
	}

	if err := mod.Shutdown(ctx); err != nil {
		t.Fatalf("persistence shutdown: %v", err)
	}

	unexpected := inspectStateDirForPlaintext(t, stateDir)
	if len(unexpected) > 0 {
		t.Fatalf("plaintext artifacts detected in state dir: %v", unexpected)
	}
}

func inspectStateDirForPlaintext(t *testing.T, root string) []string {
	t.Helper()

	var unexpected []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		allowDir := func(rel string) bool {
			if rel == "crypto" || rel == "ciphertext" || rel == "mounts" || rel == "volumes" {
				return true
			}
			if strings.HasPrefix(rel, "ciphertext"+string(filepath.Separator)) {
				return true
			}
			if strings.HasPrefix(rel, "mounts"+string(filepath.Separator)) {
				// Mount directories may exist but must be empty of files.
				return true
			}
			return false
		}

		if strings.HasPrefix(rel, "volumes"+string(filepath.Separator)) {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) == 2 && d.IsDir() {
				// volumes/<id>
				return nil
			}
			if len(parts) == 3 && parts[2] == "state.json" && !d.IsDir() {
				return nil
			}
			unexpected = append(unexpected, rel)
			return nil
		}

		if d.IsDir() {
			if !allowDir(rel) {
				unexpected = append(unexpected, rel+"/")
			}
			return nil
		}

		if rel == "crypto/keyset.json" {
			return nil
		}
		if strings.HasPrefix(rel, "ciphertext"+string(filepath.Separator)) {
			return nil
		}
		if strings.HasPrefix(rel, "mounts"+string(filepath.Separator)) {
			unexpected = append(unexpected, rel)
			return nil
		}

		unexpected = append(unexpected, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk state dir: %v", err)
	}
	sort.Strings(unexpected)
	return unexpected
}

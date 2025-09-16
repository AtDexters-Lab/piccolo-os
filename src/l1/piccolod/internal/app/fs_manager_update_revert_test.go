package app

import (
    "context"
    "os"
    "testing"
    "piccolod/internal/api"
)

func TestFSManager_UpdateImage_And_Revert(t *testing.T) {
    tmp, err := os.MkdirTemp("", "fs_update_revert")
    if err != nil { t.Fatal(err) }
    defer os.RemoveAll(tmp)

    mock := NewMockContainerManager()
    mgr, err := NewFSManager(mock, tmp)
    if err != nil { t.Fatalf("fs manager: %v", err) }
    ctx := context.Background()

    // Install initial app
    def := &api.AppDefinition{
        Name: "demoapp", Image: "alpine:3.18", Type: "user",
        Listeners: []api.AppListener{{Name:"web", GuestPort:80}},
    }
    inst, err := mgr.Install(ctx, def)
    if err != nil { t.Fatalf("install: %v", err) }
    firstCID := inst.ContainerID

    // Update tag to 3.19
    tag := "3.19"
    if err := mgr.UpdateImage(ctx, "demoapp", &tag); err != nil { t.Fatalf("update: %v", err) }

    // Verify new image written
    cur, err := mgr.stateManager.GetAppDefinition("demoapp")
    if err != nil { t.Fatalf("get def: %v", err) }
    if cur.Image != "alpine:3.19" { t.Fatalf("expected image alpine:3.19, got %s", cur.Image) }
    // Instance should have new CID
    inst2, err := mgr.Get(ctx, "demoapp")
    if err != nil { t.Fatalf("get app: %v", err) }
    if inst2.ContainerID == firstCID { t.Fatalf("expected new container id after update") }

    // Revert back to previous
    if err := mgr.Revert(ctx, "demoapp"); err != nil { t.Fatalf("revert: %v", err) }
    cur2, err := mgr.stateManager.GetAppDefinition("demoapp")
    if err != nil { t.Fatalf("get def2: %v", err) }
    // Expect image to be 3.18 again
    if cur2.Image != "alpine:3.18" { t.Fatalf("expected image alpine:3.18 after revert, got %s", cur2.Image) }
}

func TestFSManager_Logs(t *testing.T) {
    tmp, err := os.MkdirTemp("", "fs_logs")
    if err != nil { t.Fatal(err) }
    defer os.RemoveAll(tmp)
    mock := NewMockContainerManager()
    mgr, err := NewFSManager(mock, tmp)
    if err != nil { t.Fatalf("fs manager: %v", err) }
    ctx := context.Background()

    def := &api.AppDefinition{Name: "demo", Image: "alpine:latest", Type: "user", Listeners: []api.AppListener{{Name:"web", GuestPort:80}}}
    inst, err := mgr.Install(ctx, def)
    if err != nil { t.Fatalf("install: %v", err) }
    if inst.ContainerID == "" { t.Fatalf("no container id") }
    lines, err := mgr.Logs(ctx, "demo", 5)
    if err != nil { t.Fatalf("logs: %v", err) }
    if len(lines) != 5 { t.Fatalf("expected 5 lines, got %d", len(lines)) }
}


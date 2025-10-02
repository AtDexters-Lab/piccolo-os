package health

import "testing"

func TestTrackerSetAndSnapshot(t *testing.T) {
	tracker := NewTracker()
	tracker.Setf("component", LevelOK, "initialized")
	snap := tracker.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(snap))
	}
	if snap["component"].Level != LevelOK {
		t.Fatalf("expected level ok")
	}
}

func TestTrackerReady(t *testing.T) {
	tracker := NewTracker()
	tracker.Setf("a", LevelOK, "ready")
	tracker.Setf("b", LevelWarn, "partial")

	ready, _ := tracker.Ready("a")
	if !ready {
		t.Fatal("component a should be ready")
	}

	ready, _ = tracker.Ready("a", "b")
	if ready {
		t.Fatal("component b should make readiness fail")
	}
}

func TestTrackerOverall(t *testing.T) {
	tracker := NewTracker()
	tracker.Setf("a", LevelOK, "ok")
	tracker.Setf("b", LevelWarn, "warn")
	if tracker.Overall() != LevelWarn {
		t.Fatalf("expected overall warn")
	}
	tracker.Setf("c", LevelError, "fail")
	if tracker.Overall() != LevelError {
		t.Fatalf("expected overall error")
	}
}

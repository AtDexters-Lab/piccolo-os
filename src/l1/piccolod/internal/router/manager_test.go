package router

import (
	"testing"
)

func TestManagerKernelRoute(t *testing.T) {
	mgr := NewManager()
	route := mgr.KernelRoute()
	if route.Mode != ModeLocal {
		t.Fatalf("expected default kernel mode local, got %s", route.Mode)
	}

	mgr.RegisterKernelRoute(ModeTunnel, "leader-1")
	route = mgr.KernelRoute()
	if route.Mode != ModeTunnel {
		t.Fatalf("expected tunnel mode, got %s", route.Mode)
	}
	if route.LeaderAddr != "leader-1" {
		t.Fatalf("expected leader-1, got %s", route.LeaderAddr)
	}
}

func TestManagerAppRoute(t *testing.T) {
	mgr := NewManager()
	route := mgr.AppRoute("demo")
	if route.Mode != ModeLocal {
		t.Fatalf("expected default local route, got %s", route.Mode)
	}

	mgr.RegisterAppRoute("demo", ModeTunnel, "leader-b")
	route = mgr.AppRoute("demo")
	if route.Mode != ModeTunnel {
		t.Fatalf("expected tunnel mode, got %s", route.Mode)
	}
	if route.LeaderAddr != "leader-b" {
		t.Fatalf("expected leader-b, got %s", route.LeaderAddr)
	}
}

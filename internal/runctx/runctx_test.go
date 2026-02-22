package runctx

import (
	"path/filepath"
	"testing"
)

func TestDefaultRunID(t *testing.T) {
	tests := []struct {
		repoRoot string
		want     string
	}{
		{repoRoot: "/tmp/my-repo", want: "demo-it-my-repo"},
		{repoRoot: "/tmp/demo-it", want: "demo-it"},
		{repoRoot: "/tmp/demo-it-foo", want: "demo-it-foo"},
	}

	for _, tc := range tests {
		if got := DefaultRunID(tc.repoRoot); got != tc.want {
			t.Fatalf("DefaultRunID(%q) = %q, want %q", tc.repoRoot, got, tc.want)
		}
	}
}

func TestSessionNamesFromWorkspacePath(t *testing.T) {
	workspace := "/tmp/Examples/Demo Project"

	if got := SessionPrefix(workspace); got != "demo-project" {
		t.Fatalf("SessionPrefix(%q) = %q, want %q", workspace, got, "demo-project")
	}
	if got := DemoSessionName(workspace); got != "demo-project-demo" {
		t.Fatalf("DemoSessionName(%q) = %q, want %q", workspace, got, "demo-project-demo")
	}
	if got := NotesSessionName(workspace); got != "demo-project-notes" {
		t.Fatalf("NotesSessionName(%q) = %q, want %q", workspace, got, "demo-project-notes")
	}
}

func TestDefaultSocketPathUsesRuntimeDir(t *testing.T) {
	t.Setenv("DEMO_IT_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/runtime")

	got := DefaultSocketPath(filepath.Join("/home/user", "repo"))
	want := filepath.Join("/tmp/runtime", "demo-it", "demo-it-repo.sock")
	if got != want {
		t.Fatalf("DefaultSocketPath() = %q, want %q", got, want)
	}
}

func TestDefaultSocketPathUsesExplicitOverride(t *testing.T) {
	t.Setenv("DEMO_IT_SOCKET", "/tmp/custom.sock")
	got := DefaultSocketPath("/tmp/repo")
	if got != "/tmp/custom.sock" {
		t.Fatalf("DefaultSocketPath() = %q, want %q", got, "/tmp/custom.sock")
	}
}

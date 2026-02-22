package main

import (
	"reflect"
	"testing"
)

func TestTmuxKeyNormalizesReturn(t *testing.T) {
	if got := tmuxKey("return"); got != "Enter" {
		t.Fatalf("tmuxKey(return) = %q, want Enter", got)
	}
	if got := tmuxKey("Enter"); got != "Enter" {
		t.Fatalf("tmuxKey(Enter) = %q, want Enter", got)
	}
}

func TestIsProtocolCommand(t *testing.T) {
	if !isProtocolCommand("next") {
		t.Fatal("expected next to be protocol command")
	}
	if isProtocolCommand("./examples/demo") {
		t.Fatal("path should not be protocol command")
	}
}

func TestIsLegacyDemoSession(t *testing.T) {
	if !isLegacyDemoSession("demo-demo") {
		t.Fatal("expected demo-demo to be recognized")
	}
	if !isLegacyDemoSession("demo-notes") {
		t.Fatal("expected demo-notes to be recognized")
	}
	if isLegacyDemoSession("main") {
		t.Fatal("main should not be recognized")
	}
}

func TestSelectSessionsToKill(t *testing.T) {
	sessions := []string{"a-demo", "a-notes", "b-demo"}

	all, err := selectSessionsToKill(sessions, nil)
	if err != nil {
		t.Fatalf("select all: %v", err)
	}
	if !reflect.DeepEqual(all, sessions) {
		t.Fatalf("select all = %#v, want %#v", all, sessions)
	}

	some, err := selectSessionsToKill(sessions, []string{"2", "1", "2"})
	if err != nil {
		t.Fatalf("select some: %v", err)
	}
	wantSome := []string{"a-notes", "a-demo"}
	if !reflect.DeepEqual(some, wantSome) {
		t.Fatalf("select some = %#v, want %#v", some, wantSome)
	}

	if _, err := selectSessionsToKill(sessions, []string{"x"}); err == nil {
		t.Fatal("expected error for non-numeric index")
	}
	if _, err := selectSessionsToKill(sessions, []string{"9"}); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

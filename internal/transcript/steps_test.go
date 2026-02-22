package transcript

import (
	"strings"
	"testing"
)

func TestParseStepsMarkdownParsesDemoItBlocks(t *testing.T) {
	markdown := `## Intro

` + "```demo-it" + `
title: Open nvim
actions:
  - kind: insert-text
    text: nvim
  - kind: key
    key: return
speaker_notes: |
  say hello
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("ParseStepsMarkdown error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Title != "Open nvim" {
		t.Fatalf("unexpected title: %q", steps[0].Title)
	}
	if len(steps[0].Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(steps[0].Actions))
	}
}

func TestParseStepsMarkdownRejectsUnknownActionKind(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: x
actions:
  - kind: unknown
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported action kind") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownRequiresClosedBlock(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: x
actions:
  - kind: key
    key: return
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("unexpected error: %v", err)
	}
}

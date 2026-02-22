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

func TestParseStepsMarkdownSplitPaneDefaultsDirection(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: split
actions:
  - kind: split-pane
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected split-pane to parse, got %v", err)
	}
	if got := steps[0].Actions[0].Direction; got != "right" {
		t.Fatalf("split-pane default direction = %q, want right", got)
	}
}

func TestParseStepsMarkdownSplitPaneVerticalSetsDownDirection(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: split
actions:
  - kind: split-pane-vertical
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected split-pane-vertical to parse, got %v", err)
	}
	if got := steps[0].Actions[0].Direction; got != "down" {
		t.Fatalf("split-pane-vertical direction = %q, want down", got)
	}
}

func TestParseStepsMarkdownParsesClearPanes(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: clear
actions:
  - kind: clear-panes
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected clear-panes to parse, got %v", err)
	}
	if steps[0].Actions[0].Kind != "clear-panes" {
		t.Fatalf("unexpected kind: %q", steps[0].Actions[0].Kind)
	}
}

func TestParseStepsMarkdownParsesOpenSlide(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
actions:
  - kind: open-slide
    path: docs/slide.md
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected open-slide to parse, got %v", err)
	}
	if got := steps[0].Actions[0].Path; got != "docs/slide.md" {
		t.Fatalf("open-slide path = %q, want docs/slide.md", got)
	}
}

func TestParseStepsMarkdownRequiresOpenSlidePath(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
actions:
  - kind: open-slide
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "open-slide requires path") {
		t.Fatalf("unexpected error: %v", err)
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

func TestParseStepsMarkdownRejectsInvalidSplitDirection(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: split
actions:
  - kind: split-pane
    direction: left
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "split-pane direction") {
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

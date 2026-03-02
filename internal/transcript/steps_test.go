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

func TestParseStepsMarkdownParsesKillAllPane(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: clear
actions:
  - kind: killall-pane
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected killall-pane to parse, got %v", err)
	}
	if steps[0].Actions[0].Kind != "killall-pane" {
		t.Fatalf("unexpected kind: %q", steps[0].Actions[0].Kind)
	}
}

func TestParseStepsMarkdownMapsClearPanesToKillAllPane(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: clear
actions:
  - kind: clear-panes
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected clear-panes alias to parse, got %v", err)
	}
	if steps[0].Actions[0].Kind != "killall-pane" {
		t.Fatalf("unexpected mapped kind: %q", steps[0].Actions[0].Kind)
	}
}

func TestParseStepsMarkdownParsesKillPane(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: close
actions:
  - kind: kill-pane
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected kill-pane to parse, got %v", err)
	}
	if steps[0].Actions[0].Kind != "kill-pane" {
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

func TestParseStepsMarkdownParsesAutoSlideInMS(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: auto
auto_slide_in_ms: 1200
actions:
  - kind: key
    key: return
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected auto_slide_in_ms to parse, got %v", err)
	}
	if steps[0].AutoSlideInMS == nil || *steps[0].AutoSlideInMS != 1200 {
		t.Fatalf("auto_slide_in_ms = %#v, want 1200", steps[0].AutoSlideInMS)
	}
}

func TestParseStepsMarkdownRejectsNonPositiveAutoSlideInMS(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: auto
auto_slide_in_ms: 0
actions:
  - kind: key
    key: return
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "auto_slide_in_ms") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownParsesStepSlideShorthand(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
slide: slides/2-split.2
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected step slide shorthand to parse, got %v", err)
	}
	action := steps[0].Actions[0]
	if action.Kind != "open-slide" {
		t.Fatalf("step slide shorthand kind = %q, want open-slide", action.Kind)
	}
	if action.Path != "slides/2-split.md" {
		t.Fatalf("step slide shorthand path = %q, want slides/2-split.md", action.Path)
	}
}

func TestParseStepsMarkdownParsesStepSlideAndActions(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
slide: slides/intro
actions:
  - kind: split-pane
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected step slide with actions to parse, got %v", err)
	}
	if len(steps[0].Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(steps[0].Actions))
	}
	if steps[0].Actions[0].Kind != "open-slide" {
		t.Fatalf("first action = %q, want open-slide", steps[0].Actions[0].Kind)
	}
	if steps[0].Actions[1].Kind != "split-pane" {
		t.Fatalf("second action = %q, want split-pane", steps[0].Actions[1].Kind)
	}
}

func TestParseStepsMarkdownParsesKeySlideAlias(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
actions:
  - kind: key
    slide: slides/2-split.2
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected key slide alias to parse, got %v", err)
	}
	action := steps[0].Actions[0]
	if action.Kind != "open-slide" {
		t.Fatalf("key slide alias kind = %q, want open-slide", action.Kind)
	}
	if action.Path != "slides/2-split.md" {
		t.Fatalf("key slide alias path = %q, want slides/2-split.md", action.Path)
	}
}

func TestParseStepsMarkdownRejectsKeyWithSlideAndKey(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
actions:
  - kind: key
    key: return
    slide: slides/2-split.2
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "key supports key or slide") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownParsesKeyMacro(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: macro
actions:
  - kind: key-macro
    pane: last
    interval_ms: 90
    keys:
      - key: i
      - key: h
      - key: return
        delay_ms: 250
` + "```" + `
`

	steps, err := ParseStepsMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected key-macro to parse, got %v", err)
	}
	action := steps[0].Actions[0]
	if action.Kind != "key-macro" {
		t.Fatalf("unexpected kind: %q", action.Kind)
	}
	if action.Pane != "last" {
		t.Fatalf("pane = %q, want last", action.Pane)
	}
	if action.IntervalMS == nil || *action.IntervalMS != 90 {
		t.Fatalf("interval_ms = %#v, want 90", action.IntervalMS)
	}
	if len(action.Keys) != 3 {
		t.Fatalf("expected 3 macro keys, got %d", len(action.Keys))
	}
	if action.Keys[2].DelayMS == nil || *action.Keys[2].DelayMS != 250 {
		t.Fatalf("delay_ms = %#v, want 250", action.Keys[2].DelayMS)
	}
}

func TestParseStepsMarkdownRejectsKeyMacroWithoutKeys(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: macro
actions:
  - kind: key-macro
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "key-macro requires keys") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownRejectsKeyMacroNegativeDelay(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: macro
actions:
  - kind: key-macro
    keys:
      - key: a
        delay_ms: -1
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "delay_ms") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownRejectsKeyMacroNegativeInterval(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: macro
actions:
  - kind: key-macro
    interval_ms: -5
    keys:
      - key: a
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "interval_ms") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownRejectsDuplicateTitles(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: repeat
actions:
  - kind: key
    key: return
` + "```" + `

` + "```demo-it" + `
title: repeat
actions:
  - kind: key
    key: return
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate title") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseStepsMarkdownRequiresActionsOrSlide(t *testing.T) {
	markdown := `
` + "```demo-it" + `
title: open
` + "```" + `
`

	_, err := ParseStepsMarkdown(markdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing actions or slide") {
		t.Fatalf("unexpected error: %v", err)
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

func TestSlideReferenceToPathAddsMarkdownExtension(t *testing.T) {
	if got := slideReferenceToPath("slides/intro"); got != "slides/intro.md" {
		t.Fatalf("slideReferenceToPath = %q, want slides/intro.md", got)
	}
	if got := slideReferenceToPath("slides/2-split.2"); got != "slides/2-split.md" {
		t.Fatalf("slideReferenceToPath numeric alias = %q, want slides/2-split.md", got)
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

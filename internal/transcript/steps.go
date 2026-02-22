package transcript

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Action struct {
	Kind      string `yaml:"kind"`
	Text      string `yaml:"text,omitempty"`
	Key       string `yaml:"key,omitempty"`
	Slide     string `yaml:"slide,omitempty"`
	Direction string `yaml:"direction,omitempty"`
	Path      string `yaml:"path,omitempty"`
}

type Step struct {
	Title        string   `yaml:"title"`
	Slide        string   `yaml:"slide,omitempty"`
	Actions      []Action `yaml:"actions"`
	SpeakerNotes string   `yaml:"speaker_notes,omitempty"`
}

func ParseStepsFile(path string) ([]Step, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseStepsMarkdown(string(data))
}

func ParseStepsMarkdown(markdown string) ([]Step, error) {
	lines := strings.Split(markdown, "\n")
	insideBlock := false
	blockStart := 0
	var blockLines []string
	steps := make([]Step, 0)

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !insideBlock {
			if trimmed == "```demo-it" {
				insideBlock = true
				blockStart = idx + 1
				blockLines = blockLines[:0]
			}
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			step, err := decodeStep(strings.Join(blockLines, "\n"), blockStart)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
			insideBlock = false
			continue
		}

		blockLines = append(blockLines, line)
	}

	if insideBlock {
		return nil, fmt.Errorf("unterminated demo-it block starting at line %d", blockStart)
	}

	return steps, nil
}

func decodeStep(raw string, line int) (Step, error) {
	var step Step
	if err := yaml.Unmarshal([]byte(raw), &step); err != nil {
		return Step{}, fmt.Errorf("decode demo-it block at line %d: %w", line, err)
	}

	if strings.TrimSpace(step.Title) == "" {
		return Step{}, fmt.Errorf("demo-it block at line %d: missing title", line)
	}

	stepSlide := strings.TrimSpace(step.Slide)
	if stepSlide != "" {
		step.Actions = append([]Action{{Kind: "open-slide", Path: slideReferenceToPath(stepSlide)}}, step.Actions...)
		step.Slide = ""
	}
	if len(step.Actions) == 0 {
		return Step{}, fmt.Errorf("demo-it block at line %d: missing actions or slide", line)
	}

	for i, action := range step.Actions {
		kind := strings.TrimSpace(action.Kind)
		action.Kind = kind
		switch kind {
		case "insert-text":
			if strings.TrimSpace(action.Text) == "" {
				return Step{}, fmt.Errorf("demo-it block at line %d: insert-text requires text", line)
			}
		case "key":
			slide := strings.TrimSpace(action.Slide)
			if slide != "" {
				if strings.TrimSpace(action.Key) != "" {
					return Step{}, fmt.Errorf("demo-it block at line %d: key supports key or slide, not both", line)
				}
				action.Kind = "open-slide"
				action.Path = slideReferenceToPath(slide)
				action.Slide = ""
				break
			}
			if strings.TrimSpace(action.Key) == "" {
				return Step{}, fmt.Errorf("demo-it block at line %d: key requires key", line)
			}
		case "split-pane":
			direction := strings.TrimSpace(action.Direction)
			if direction == "" {
				action.Direction = "right"
			} else {
				switch direction {
				case "right", "down":
					action.Direction = direction
				default:
					return Step{}, fmt.Errorf("demo-it block at line %d: split-pane direction must be right|down", line)
				}
			}
		case "split-pane-vertical":
			action.Direction = "down"
		case "clear-panes":
			// no extra args
		case "open-slide":
			if strings.TrimSpace(action.Path) == "" {
				return Step{}, fmt.Errorf("demo-it block at line %d: open-slide requires path", line)
			}
		default:
			return Step{}, fmt.Errorf("demo-it block at line %d: unsupported action kind %q", line, action.Kind)
		}
		step.Actions[i] = action
	}

	return step, nil
}

func slideReferenceToPath(reference string) string {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".md") {
		return trimmed
	}

	lastSlash := strings.LastIndex(trimmed, "/")
	lastDot := strings.LastIndex(trimmed, ".")
	if lastDot > lastSlash {
		suffix := trimmed[lastDot+1:]
		if isNumeric(suffix) {
			return trimmed[:lastDot] + ".md"
		}
		return trimmed
	}
	return trimmed + ".md"
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

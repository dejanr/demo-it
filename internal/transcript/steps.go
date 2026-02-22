package transcript

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Action struct {
	Kind string `yaml:"kind"`
	Text string `yaml:"text,omitempty"`
	Key  string `yaml:"key,omitempty"`
}

type Step struct {
	Title        string   `yaml:"title"`
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
	if len(step.Actions) == 0 {
		return Step{}, fmt.Errorf("demo-it block at line %d: missing actions", line)
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
			if strings.TrimSpace(action.Key) == "" {
				return Step{}, fmt.Errorf("demo-it block at line %d: key requires key", line)
			}
		default:
			return Step{}, fmt.Errorf("demo-it block at line %d: unsupported action kind %q", line, action.Kind)
		}
		step.Actions[i] = action
	}

	return step, nil
}

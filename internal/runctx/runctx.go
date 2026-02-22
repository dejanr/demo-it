package runctx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9._-]+`)

func RepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return findRepoRoot(cwd), nil
}

func findRepoRoot(start string) string {
	current := start
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return start
		}
		current = parent
	}
}

func DefaultRunID(repoRoot string) string {
	slug := slugFromPath(repoRoot, "run")
	if slug == "demo-it" || strings.HasPrefix(slug, "demo-it-") {
		return slug
	}
	return "demo-it-" + slug
}

func SessionPrefix(workspacePath string) string {
	return slugFromPath(workspacePath, "demo")
}

func DemoSessionName(workspacePath string) string {
	return SessionPrefix(workspacePath) + "-demo"
}

func NotesSessionName(workspacePath string) string {
	return SessionPrefix(workspacePath) + "-notes"
}

func DefaultSocketPath(repoRoot string) string {
	if explicit := os.Getenv("DEMO_IT_SOCKET"); explicit != "" {
		return explicit
	}

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}

	return filepath.Join(runtimeDir, "demo-it", DefaultRunID(repoRoot)+".sock")
}

func slugFromPath(path string, fallback string) string {
	base := filepath.Base(path)
	slug := strings.ToLower(base)
	slug = nonSlugChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" || slug == "." {
		slug = fallback
	}
	return slug
}

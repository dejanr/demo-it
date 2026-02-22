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
	return cwd, nil
}

func DefaultRunID(repoRoot string) string {
	base := filepath.Base(repoRoot)
	slug := strings.ToLower(base)
	slug = nonSlugChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "run"
	}
	if slug == "demo-it" || strings.HasPrefix(slug, "demo-it-") {
		return slug
	}
	return "demo-it-" + slug
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

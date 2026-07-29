package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the configuration's name within the airfs configuration
// directory.
const FileName = "config.yaml"

// DefaultPath is where the configuration lives when no override is given:
// $XDG_CONFIG_HOME/airfs/config.yaml, falling back to ~/.config/airfs when
// XDG_CONFIG_HOME is unset.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "airfs", FileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "airfs", FileName), nil
}

// Expand applies the leading `~`, then `$VAR` and `${VAR}`, then resolves a
// still-relative path against base — the directory holding the configuration
// file. An unset variable is an error, never an empty string: silently
// resolving to a shorter path would layer, or expose, something nobody
// declared.
//
// An empty base leaves a relative path relative, which is what a frontend
// deciding whether a path needs making absolute before it is written down
// asks about.
//
// It is exported so that a frontend writing a declaration into the file can
// ask what it will mean when read back, rather than reimplementing these
// rules.
func Expand(path, base string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~ in %s: %w", path, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	expanded, err := expandVars(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		if base == "" {
			return filepath.Clean(expanded), nil
		}
		expanded = filepath.Join(base, expanded)
	}
	return filepath.Clean(expanded), nil
}

// expandVars substitutes $VAR and ${VAR} from the environment, failing on the
// first name that is not set.
func expandVars(s string) (string, error) {
	var missing string
	out := os.Expand(s, func(name string) string {
		value, ok := os.LookupEnv(name)
		if !ok && missing == "" {
			missing = name
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("%s: $%s is not set", s, missing)
	}
	return out, nil
}

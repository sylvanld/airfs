// Package sources reads the source list and resolves it into the ordered
// layers a merged view is built from. See docs/specs/source-config.md.
package sources

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sylvanld/airfs/sdk"
	"github.com/sylvanld/airfs/sdk/layerfs"
)

// FileName is the configuration file's name at the root of a target.
const FileName = "sources.txt"

// A Source is one contributing directory tree.
//
// Declared is the path as the author wrote it, which is what reports name;
// Path is that path expanded and made absolute, which is what is read. A
// symlinked source is followed on read but still reported as declared.
type Source struct {
	Declared string
	Path     string
	Line     int
}

// A Config is a resolved source list, in precedence order: later sources win.
type Config struct {
	// Path is the configuration file this was read from.
	Path string
	// Sources are the declared sources, in declared order.
	Sources []Source
}

// Load reads and resolves the configuration at path.
//
// Resolution confirms that every source directory exists and creates the kind
// subdirectories that are missing, so that every source contributes every kind
// and the stack is uniform. It enumerates no entries: that is the merge's work.
func Load(path string) (*Config, error) {
	cfg, err := Parse(path)
	if err != nil {
		return nil, err
	}
	for _, s := range cfg.Sources {
		info, err := os.Stat(s.Path)
		if err != nil {
			return nil, airfs.Precondition(fmt.Errorf(
				"%s:%d: source %s: %w", path, s.Line, s.Declared, err))
		}
		if !info.IsDir() {
			return nil, airfs.Precondition(fmt.Errorf(
				"%s:%d: source %s is not a directory", path, s.Line, s.Declared))
		}
		for _, kind := range airfs.Kinds {
			if err := os.Mkdir(filepath.Join(s.Path, kind.String()), 0o755); err != nil && !os.IsExist(err) {
				return nil, fmt.Errorf("source %s: creating %s: %w", s.Declared, kind, err)
			}
		}
	}
	return cfg, nil
}

// Parse reads path and expands every declaration, without touching the
// declared directories. Load is what callers normally want; Parse exists so
// that the file's syntax can be exercised on its own.
func Parse(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, airfs.Precondition(err)
	}
	defer file.Close()

	base := filepath.Dir(path)
	cfg := &Config{Path: path}
	seen := make(map[string]Source)

	scan := bufio.NewScanner(file)
	for line := 1; scan.Scan(); line++ {
		declared, ok := meaningful(scan.Text())
		if !ok {
			continue
		}
		resolved, err := Expand(declared, base)
		if err != nil {
			return nil, airfs.Precondition(fmt.Errorf("%s:%d: %w", path, line, err))
		}
		if first, dup := seen[resolved]; dup {
			return nil, airfs.Precondition(fmt.Errorf(
				"%s:%d: %s duplicates line %d (%s); both resolve to %s",
				path, line, declared, first.Line, first.Declared, resolved))
		}
		s := Source{Declared: declared, Path: resolved, Line: line}
		seen[resolved] = s
		cfg.Sources = append(cfg.Sources, s)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// meaningful strips a line's comment and surrounding whitespace, and reports
// whether anything is left to declare.
func meaningful(line string) (string, bool) {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	return line, line != ""
}

// Expand applies the leading `~`, then `$VAR` and `${VAR}`, then resolves a
// still-relative path against base — the directory holding the configuration
// file. An unset variable is an error, never an empty string: silently
// resolving to a shorter path would layer something nobody declared.
//
// It is exported so that a frontend writing a declaration into the file can ask
// what it will mean when read back, rather than reimplementing these rules.
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

// Layers builds the ordered layers a kind's merged view is composed of: one
// stratum per source, in declared order, named as the author declared it.
func (c *Config) Layers(kind airfs.Kind) []layerfs.Layer {
	layers := make([]layerfs.Layer, 0, len(c.Sources))
	for _, s := range c.Sources {
		root := filepath.Join(s.Path, kind.String())
		layers = append(layers, layerfs.Layer{
			Name: s.Declared,
			FS:   os.DirFS(root),
			Root: root,
		})
	}
	return layers
}

// Merged is the union for one kind, over every source's stratum.
func (c *Config) Merged(kind airfs.Kind) *layerfs.FS {
	return layerfs.New(c.Layers(kind)...)
}

// Counts reports how many entries each source contributes to kind, indexed the
// same way as Sources. Counting enumerates the strata, which is the merge's
// work rather than resolution's, so it is done here on demand.
func (c *Config) Counts(kind airfs.Kind) ([]int, error) {
	counts := make([]int, len(c.Sources))
	for i, layer := range c.Layers(kind) {
		entries, err := fs.ReadDir(layer.FS, ".")
		if err != nil {
			return nil, err
		}
		counts[i] = len(entries)
	}
	return counts, nil
}

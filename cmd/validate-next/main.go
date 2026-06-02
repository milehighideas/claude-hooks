// validate-next is a standalone validator for Next.js projects. It verifies
// that public/ asset references and internal links resolve, reading its
// configuration from .pre-commit.json (the same nextImageCheck / nextLinkCheck
// blocks the pre-commit orchestrator uses). The detection logic is shared via
// internal/nextchecks, so this binary and `pre-commit --check next*` never drift.
//
// Exit codes: 0 = clean, 1 = error running checks, 2 = unresolved references.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
	"github.com/milehighideas/claude-hooks/internal/nextchecks"
)

var (
	pathFlag  string
	checkFlag string
	helpFlag  bool
)

func init() {
	flag.StringVar(&pathFlag, "path", ".", "Project directory to check (where .pre-commit.json lives or below)")
	flag.StringVar(&checkFlag, "check", "both", "Which checks to run: images | links | both")
	flag.BoolVar(&helpFlag, "help", false, "Show this help message")
	flag.BoolVar(&helpFlag, "h", false, "Show this help message")
}

type appCfg struct {
	Path string `json:"path"`
}

type imageBlock struct {
	Apps []string `json:"apps"`
	nextchecks.ImageConfig
}

type linkBlock struct {
	Apps []string `json:"apps"`
	nextchecks.LinkConfig
}

type preCommitConfig struct {
	Apps           map[string]appCfg `json:"apps"`
	NextImageCheck imageBlock        `json:"nextImageCheck"`
	NextLinkCheck  linkBlock         `json:"nextLinkCheck"`
}

func printUsage() {
	fmt.Println("validate-next - public-asset & internal-link validator for Next.js")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  validate-next --path <dir> [--check images|links|both]")
	fmt.Println()
	fmt.Println("FLAGS:")
	fmt.Println("  -path <dir>    Project dir containing (or below) .pre-commit.json (default \".\")")
	fmt.Println("  -check <what>  images | links | both (default \"both\")")
	fmt.Println("  -h, -help      Show this help message")
	fmt.Println()
	fmt.Println("Config: reads the nextImageCheck / nextLinkCheck blocks from .pre-commit.json.")
	fmt.Println()
	fmt.Println("EXIT CODES:")
	fmt.Println("  0 - No unresolved references")
	fmt.Println("  1 - Error running checks")
	fmt.Println("  2 - Unresolved references found")
}

func main() {
	flag.Parse()
	if helpFlag {
		printUsage()
		os.Exit(0)
	}
	os.Exit(run())
}

func run() int {
	configDir, cfg, err := loadConfig(pathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate-next: %v\n", err)
		return 1
	}

	failed := false
	if checkFlag == "images" || checkFlag == "both" {
		if reportNext(configDir, cfg.Apps, cfg.NextImageCheck.Apps, "image reference",
			func(appPath string) (nextchecks.Result, error) {
				return nextchecks.CheckImages(appPath, cfg.NextImageCheck.WithDefaults())
			}) {
			failed = true
		}
	}
	if checkFlag == "links" || checkFlag == "both" {
		if reportNext(configDir, cfg.Apps, cfg.NextLinkCheck.Apps, "link",
			func(appPath string) (nextchecks.Result, error) {
				return nextchecks.CheckLinks(appPath, cfg.NextLinkCheck.WithDefaults())
			}) {
			failed = true
		}
	}
	if checkFlag != "images" && checkFlag != "links" && checkFlag != "both" {
		fmt.Fprintf(os.Stderr, "validate-next: unknown --check %q (want images|links|both)\n", checkFlag)
		return 1
	}

	if failed {
		return 2
	}
	return 0
}

// reportNext runs fn across the in-scope apps and prints results. Returns true
// if any unresolved references were found.
func reportNext(
	configDir string,
	apps map[string]appCfg,
	scope []string,
	noun string,
	fn func(appPath string) (nextchecks.Result, error),
) bool {
	type run struct{ name, path string }
	var runs []run
	if len(scope) == 0 {
		for name, a := range apps {
			runs = append(runs, run{name, a.Path})
		}
	} else {
		for _, name := range scope {
			if a, ok := apps[name]; ok {
				runs = append(runs, run{name, a.Path})
			}
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].name < runs[j].name })

	total := 0
	for _, r := range runs {
		appPath := filepath.Join(configDir, r.path)
		res, err := fn(appPath)
		if err != nil {
			fmt.Printf("⚠️  %s: %v\n", r.name, err)
			continue
		}
		if res.Skipped {
			fmt.Printf("⏩ %s skipped (%s)\n", r.name, res.Reason)
			continue
		}
		if len(res.Misses) == 0 {
			fmt.Printf("✓ %s — %d %ss OK\n", r.name, res.Scanned, noun)
			continue
		}
		for _, m := range res.Misses {
			fmt.Printf("✗ %s  %s  (in %s)\n", r.name, m.Ref, m.File)
			total++
		}
	}
	if total > 0 {
		fmt.Printf("\nFound %d unresolved %s(s)\n", total, noun)
	}
	return total > 0
}

// loadConfig walks up from start to find .pre-commit.json and parses it.
func loadConfig(start string) (string, *preCommitConfig, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", nil, err
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		candidate := filepath.Join(dir, ".pre-commit.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			var cfg preCommitConfig
			if err := jsonc.Unmarshal(candidate, &cfg); err != nil {
				return "", nil, fmt.Errorf("parse %s: %w", candidate, err)
			}
			return dir, &cfg, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, fmt.Errorf("no .pre-commit.json found from %s upward", abs)
		}
		dir = parent
	}
}

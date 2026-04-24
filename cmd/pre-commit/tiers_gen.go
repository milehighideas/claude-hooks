package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// tiersGenConfigFile is the per-project config that names the watched tier
// config file and the regeneration command to invoke. Mirrors the schema
// consumed by the auto-tiers-gen PostToolUse hook so edit-time and commit-time
// paths share one source of truth.
const tiersGenConfigFile = ".tiers-gen.json"

type tiersGenConfig struct {
	WatchFile string   `json:"watchFile"`
	Command   []string `json:"command"`
}

// checkTiersGen runs the project's tiers-gen command when .tiers-gen.json is
// present and the watched file appears in the staged file set. Stages the
// regenerated output back into the commit so the hook is invisible on success.
//
// No-op when .tiers-gen.json is missing, malformed, or when the watched file
// isn't in the current commit.
func checkTiersGen(projectRoot string, stagedFiles []string) error {
	cfgPath := filepath.Join(projectRoot, tiersGenConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", tiersGenConfigFile, err)
	}

	var cfg tiersGenConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing %s: %w", tiersGenConfigFile, err)
	}

	if cfg.WatchFile == "" || len(cfg.Command) == 0 {
		return nil
	}

	watched := false
	for _, f := range stagedFiles {
		rel := f
		if filepath.IsAbs(f) {
			if r, err := filepath.Rel(projectRoot, f); err == nil {
				rel = r
			}
		}
		if filepath.Clean(rel) == filepath.Clean(cfg.WatchFile) {
			watched = true
			break
		}
	}
	if !watched {
		return nil
	}

	if err := runTiersGenCommand(projectRoot, cfg.Command); err != nil {
		return err
	}

	// Stage whatever the command (re)wrote — git add is a no-op if nothing changed.
	stage := exec.Command("git", "add", "--update")
	stage.Dir = projectRoot
	_ = stage.Run()

	return nil
}

func runTiersGenCommand(projectRoot string, command []string) error {
	bin, err := exec.LookPath(command[0])
	if err != nil {
		return fmt.Errorf("tiers-gen command not found: %s", command[0])
	}
	cmd := exec.Command(bin, command[1:]...)
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tiers-gen command failed: %w", err)
	}
	return nil
}

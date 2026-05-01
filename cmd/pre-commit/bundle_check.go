package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// BundleCheckConfig configures the Metro bundle-only check. Each app listed
// runs the configured npm script to bundle the JS without compiling native
// code. Catches the bug class where source code imports a package that isn't
// declared in package.json — typecheck and lint pass because the dep is
// hoisted into root node_modules from another workspace, but Metro fails to
// resolve it at runtime.
type BundleCheckConfig struct {
	// Apps is the set of app names (keys in the top-level "apps" map) whose
	// bundle:check script should be invoked. Apps not listed here are skipped.
	Apps []string `json:"apps"`
	// Script is the npm script to run inside each app. Defaults to
	// "bundle:check" when empty so existing repos don't need to set it.
	Script string `json:"script,omitempty"`
}

// bundleScriptRunner is the indirection point for tests. Production code
// invokes runBundleScript; tests overwrite this to inject deterministic
// behavior.
var bundleScriptRunner = runBundleScript

// runBundleCheck runs the configured bundle script per app in parallel and
// reports per-app pass/fail. Returns an error if any app's script exits non-zero.
func runBundleCheck(config BundleCheckConfig, apps map[string]AppConfig, packageManager string) error {
	if len(config.Apps) == 0 {
		return nil
	}

	script := config.Script
	if script == "" {
		script = "bundle:check"
	}

	pm := packageManager
	if pm == "" {
		pm = "pnpm"
	}

	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  BUNDLE CHECK (PARALLEL)")
		fmt.Println("================================")
		fmt.Printf("Bundling %d app(s) with `%s run %s` in parallel...\n\n", len(config.Apps), pm, script)
	}

	type result struct {
		app    string
		output string
		err    error
	}

	var wg sync.WaitGroup
	results := make([]result, len(config.Apps))

	for i, appName := range config.Apps {
		appCfg, ok := apps[appName]
		if !ok {
			results[i] = result{app: appName, err: fmt.Errorf("app %q not found in configuration", appName)}
			continue
		}

		wg.Add(1)
		go func(idx int, name string, path string) {
			defer wg.Done()
			out, err := bundleScriptRunner(path, pm, script)
			results[idx] = result{app: name, output: out, err: err}
		}(i, appName, appCfg.Path)
	}

	wg.Wait()

	var failed []result
	for _, r := range results {
		if r.err != nil {
			failed = append(failed, r)
			continue
		}
		if !compactMode() {
			fmt.Printf("   ✓ %s passed bundle check\n", r.app)
		}
	}

	if len(failed) == 0 {
		return nil
	}

	if !compactMode() {
		for _, r := range failed {
			fmt.Printf("\n   ❌ %s bundle check failed\n", r.app)
			if r.output != "" {
				fmt.Println(strings.TrimRight(r.output, "\n"))
			}
		}
	}

	parts := make([]string, 0, len(failed))
	for _, r := range failed {
		parts = append(parts, r.app+" failed")
	}
	return fmt.Errorf("bundle check failed: %s", strings.Join(parts, ", "))
}

// runBundleScript invokes the configured package-manager command in the given
// app directory and returns combined stdout+stderr along with the exit error.
func runBundleScript(appPath, packageManager, script string) (string, error) {
	abs, err := filepath.Abs(appPath)
	if err != nil {
		return "", err
	}

	args := []string{"run", script}
	cmd := exec.Command(packageManager, args...)
	cmd.Dir = abs

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

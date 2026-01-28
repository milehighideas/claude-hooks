package main

import (
	"testing"
)

func TestDestructivePatterns(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// === Allowed commands ===
		{"git status", "git status", false},
		{"git add file", "git add file.go", false},
		{"git add all", "git add .", false},
		{"git commit", "git commit -m 'message'", false},
		{"git diff", "git diff", false},
		{"git log", "git log --oneline", false},
		{"git show", "git show HEAD", false},
		{"git branch list", "git branch", false},
		{"git branch create", "git branch feature-x", false},
		{"git branch delete lowercase", "git branch -d feature-x", false},
		{"git rm cached", "git rm --cached file.go", false},
		{"git fetch", "git fetch origin", false},
		{"git pull", "git pull origin main", false},
		{"git push normal", "git push origin main", false},
		{"git stash list", "git stash list", false},
		{"git stash show", "git stash show", false},

		// === git reset ===
		{"git reset soft", "git reset --soft HEAD~1", true},
		{"git reset hard", "git reset --hard HEAD", true},
		{"git reset mixed", "git reset HEAD~1", true},
		{"git reset file", "git reset file.go", true},

		// === git restore ===
		{"git restore file", "git restore file.go", true},
		{"git restore staged", "git restore --staged file.go", true},

		// === git revert ===
		{"git revert commit", "git revert abc123", true},
		{"git revert head", "git revert HEAD", true},

		// === git checkout - all blocked ===
		{"git checkout file", "git checkout -- file.go", true},
		{"git checkout dot", "git checkout .", true},
		{"git checkout branch", "git checkout main", true},
		{"git checkout path with slash", "git checkout packages/ui/src/styles/globals.css", true},
		{"git checkout nested path", "git checkout src/components/Button.tsx", true},
		{"git checkout simple path", "git checkout path/file.go", true},
		{"git checkout -b branch", "git checkout -b feature/new-branch", true},

		// === git clean ===
		{"git clean dry run", "git clean -n", true},
		{"git clean force", "git clean -fd", true},

		// === git stash operations ===
		{"git stash bare", "git stash", true},
		{"git stash chained with &&", "git stash && git add .", true},
		{"git stash chained with ;", "git stash; git add .", true},
		{"git stash chained with |", "git stash | cat", true},
		{"git stash push", "git stash push", true},
		{"git stash pop", "git stash pop", true},
		{"git stash pop chained", "git stash pop && git status", true},
		{"git stash drop", "git stash drop", true},
		{"git stash clear", "git stash clear", true},
		{"git stash apply", "git stash apply", true},
		{"git stash save", "git stash save 'message'", true},
		{"git stash with flag", "git stash -u", true},
		{"git stash with long flag", "git stash --include-untracked", true},

		// === git push force ===
		{"git push force long", "git push --force origin main", true},
		{"git push force short", "git push -f origin main", true},
		{"git push force lease", "git push --force-with-lease origin main", true},

		// === git branch force delete ===
		{"git branch force delete", "git branch -D feature-x", true},

		// === git rm ===
		{"git rm file", "git rm file.go", true},
		{"git rm recursive", "git rm -r folder/", true},

		// === git rebase ===
		{"git rebase main", "git rebase main", true},
		{"git rebase interactive", "git rebase -i HEAD~3", true},
		{"git rebase onto", "git rebase --onto main feature", true},

		// === git commit amend ===
		{"git commit amend", "git commit --amend", true},
		{"git commit amend message", "git commit --amend -m 'new message'", true},
		{"git commit amend no edit", "git commit --amend --no-edit", true},

		// === git filter-branch/repo ===
		{"git filter-branch", "git filter-branch --tree-filter 'rm -f password.txt' HEAD", true},
		{"git filter-repo", "git filter-repo --path src/", true},

		// === git reflog ===
		{"git reflog expire", "git reflog expire --expire=now --all", true},
		{"git reflog delete", "git reflog delete HEAD@{1}", true},
		{"git reflog show allowed", "git reflog show", false},
		{"git reflog bare allowed", "git reflog", false},

		// === git gc ===
		{"git gc prune now", "git gc --prune=now", true},
		{"git gc prune date", "git gc --prune=2.weeks.ago", true},
		{"git gc normal allowed", "git gc", false},

		// === git update-ref ===
		{"git update-ref delete short", "git update-ref -d HEAD", true},
		{"git update-ref delete long", "git update-ref --delete refs/heads/branch", true},

		// === git switch - all blocked ===
		{"git switch discard", "git switch --discard-changes main", true},
		{"git switch force short", "git switch -f main", true},
		{"git switch force long", "git switch --force main", true},
		{"git switch normal", "git switch main", true},
		{"git switch create", "git switch -c new-branch", true},

		// === git cherry-pick/merge abort ===
		{"git cherry-pick abort", "git cherry-pick --abort", true},
		{"git merge abort", "git merge --abort", true},
		{"git cherry-pick normal allowed", "git cherry-pick abc123", false},
		{"git merge normal allowed", "git merge feature", false},

		// === git worktree ===
		{"git worktree remove force", "git worktree remove --force /path", true},
		{"git worktree remove force short", "git worktree remove -f /path", true},
		{"git worktree add allowed", "git worktree add /path branch", false},
		{"git worktree list allowed", "git worktree list", false},

		// === git submodule ===
		{"git submodule deinit force", "git submodule deinit --force submod", true},
		{"git submodule deinit force short", "git submodule deinit -f submod", true},
		{"git submodule update allowed", "git submodule update --init", false},

		// === rm .git ===
		{"rm rf .git", "rm -rf .git", true},
		{"rm fr .git", "rm -fr .git", true},
		{"rm rf path .git", "rm -rf /path/to/.git", true},
		{"rm .git/index", "rm .git/index", true},
		{"rm .git/index.lock", "rm .git/index.lock", true},
		{"rm .git/HEAD.lock", "rm .git/HEAD.lock", true},
		{"rm .git/config", "rm .git/config", true},
		{"rm normal file allowed", "rm file.go", false},
		{"rm rf normal allowed", "rm -rf node_modules", false},

		// === Case insensitivity ===
		{"GIT RESET uppercase", "GIT RESET --hard", true},
		{"Git Reset mixed", "Git Reset HEAD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, _ := checkDestructive(tt.command)
			if blocked != tt.blocked {
				t.Errorf("command %q: got blocked=%v, want blocked=%v", tt.command, blocked, tt.blocked)
			}
		})
	}
}

func TestHookBypassPatterns(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// === Allowed ===
		{"normal commit", "git commit -m 'message'", false},
		{"normal push", "git push origin main", false},
		{"normal merge", "git merge feature", false},
		{"unrelated env var", "FOO=bar git status", false},

		// === Environment variables ===
		{"SKIP_PRECOMMIT_CHECKS", "SKIP_PRECOMMIT_CHECKS=1 git commit -m 'msg'", true},
		{"SKIP_PRE_COMMIT", "SKIP_PRE_COMMIT=true git commit -m 'msg'", true},
		{"SKIP_HOOKS", "SKIP_HOOKS=1 git commit -m 'msg'", true},
		{"SKIP_HOOK singular", "SKIP_HOOK=1 git commit -m 'msg'", true},
		{"SKIP_TESTS", "SKIP_TESTS=1 npm test", true},
		{"HUSKY=0", "HUSKY=0 git commit -m 'msg'", true},
		{"HUSKY_SKIP_HOOKS", "HUSKY_SKIP_HOOKS=1 git commit -m 'msg'", true},
		{"PRE_COMMIT_ALLOW_NO_CONFIG", "PRE_COMMIT_ALLOW_NO_CONFIG=1 git commit -m 'msg'", true},

		// === Git flags ===
		{"commit no-verify", "git commit --no-verify -m 'msg'", true},
		{"commit -n", "git commit -n -m 'msg'", true},
		{"push no-verify", "git push --no-verify origin main", true},
		{"merge no-verify", "git merge --no-verify feature", true},

		// === Git global flags with hook bypass ===
		{"commit no-verify with -C", "git -C ../.. commit --no-verify -m 'msg'", true},
		{"commit -n with -C", "git -C /path/to/repo commit -n -m 'msg'", true},
		{"push no-verify with -C", "git -C ../.. push --no-verify origin main", true},
		{"merge no-verify with -C", "git -C ../.. merge --no-verify feature", true},

		// === Combined bypass attempts ===
		{"triple bypass", "SKIP_TESTS=1 SKIP_HOOK=1 git commit --no-verify", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, _ := checkBypass(tt.command)
			if blocked != tt.blocked {
				t.Errorf("command %q: got blocked=%v, want blocked=%v", tt.command, blocked, tt.blocked)
			}
		})
	}
}

// checkDestructive checks if a command matches any destructive pattern.
// Returns true if blocked, along with the pattern name.
func checkDestructive(cmd string) (bool, string) {
	for _, p := range destructivePatterns {
		if p.regex.MatchString(cmd) {
			if p.exclude != nil && p.exclude.MatchString(cmd) {
				continue
			}
			return true, p.name
		}
	}
	return false, ""
}

// checkBypass checks if a command matches any hook bypass pattern.
// Returns true if blocked, along with the pattern name.
func checkBypass(cmd string) (bool, string) {
	for _, p := range hookBypassPatterns {
		if p.regex.MatchString(cmd) {
			if p.exclude != nil && p.exclude.MatchString(cmd) {
				continue
			}
			return true, p.name
		}
	}
	return false, ""
}

func BenchmarkPatternMatching(b *testing.B) {
	commands := []string{
		"git status",
		"git add .",
		"git commit -m 'test message'",
		"git push origin main",
		"git reset --hard HEAD",
		"SKIP_HOOKS=1 git commit -m 'msg'",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := commands[i%len(commands)]
		checkDestructive(cmd)
		checkBypass(cmd)
	}
}

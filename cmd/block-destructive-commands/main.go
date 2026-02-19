// block-destructive-commands is a Claude Code PreToolUse hook that blocks
// dangerous git commands, repository destruction, and hook bypass attempts before they execute.
//
// Exit codes:
//   - 0: Allow the command
//   - 2: Block the command (prints reason to stderr)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// pattern represents a blocked command pattern with its compiled regex and description.
type pattern struct {
	regex   *regexp.Regexp
	name    string
	exclude *regexp.Regexp // If set, pattern doesn't match when exclude also matches
}

// hookInput represents the JSON structure from Claude Code's PreToolUse hook.
// Claude Code sends nested JSON: {"tool_input": {"command": "git status"}, ...}
type hookInput struct {
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	Command string `json:"command"` // fallback for flat format (testing)
}

// destructivePatterns contains patterns that can cause catastrophic data loss or system damage.
var destructivePatterns = []pattern{
	// git reset - all forms can lose work
	{regex: regexp.MustCompile(`(?i)\bgit\s+reset\b`), name: "git reset"},

	// git restore - discards uncommitted changes
	{regex: regexp.MustCompile(`(?i)\bgit\s+restore\b`), name: "git restore"},

	// git revert - modifies history unexpectedly
	{regex: regexp.MustCompile(`(?i)\bgit\s+revert\b`), name: "git revert"},

	// git checkout - all forms blocked, user must do it manually
	{regex: regexp.MustCompile(`(?i)\bgit\s+checkout\b`), name: "git checkout (user must run manually)"},

	// git clean - removes untracked files
	{regex: regexp.MustCompile(`(?i)\bgit\s+clean\b`), name: "git clean"},

	// git stash - all stash operations can disrupt workflow
	// Match bare "git stash" at end of command OR followed by && or ; or |
	{regex: regexp.MustCompile(`(?i)\bgit\s+stash\s*($|[;&|])`), name: "git stash (bare command)"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+stash\s+(push|drop|clear|pop|apply|save|branch|create|store)`), name: "git stash subcommands"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+stash\s+--`), name: "git stash with flags"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+stash\s+-[a-zA-Z]`), name: "git stash with flags"},

	// git push --force - rewrites remote history
	{regex: regexp.MustCompile(`(?i)\bgit\s+push\s+.*--force`), name: "git push --force"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+push\s+-f\b`), name: "git push -f"},

	// git branch -D - force deletes branch (case-sensitive: -D is force, -d is safe)
	{regex: regexp.MustCompile(`\bgit\s+branch\s+.*-D\b`), name: "git branch -D (force delete)"},

	// git rm without --cached (deletes files)
	{regex: regexp.MustCompile(`(?i)\bgit\s+rm\b`), name: "git rm (use --cached to keep files)", exclude: regexp.MustCompile(`(?i)--cached`)},

	// === History Rewriting ===

	// git rebase - rewrites commit history, can lose work during conflicts
	{regex: regexp.MustCompile(`(?i)\bgit\s+rebase\b`), name: "git rebase"},

	// git commit --amend - rewrites the last commit
	{regex: regexp.MustCompile(`(?i)\bgit\s+commit\s+.*--amend\b`), name: "git commit --amend"},

	// git filter-branch / git filter-repo - rewrites entire repository history
	{regex: regexp.MustCompile(`(?i)\bgit\s+filter-branch\b`), name: "git filter-branch"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+filter-repo\b`), name: "git filter-repo"},

	// === Recovery Destruction ===

	// git reflog - expire/delete removes safety net for recovering commits
	{regex: regexp.MustCompile(`(?i)\bgit\s+reflog\s+(expire|delete)\b`), name: "git reflog expire/delete"},

	// git gc --prune - permanently removes unreachable objects
	{regex: regexp.MustCompile(`(?i)\bgit\s+gc\s+.*--prune`), name: "git gc --prune"},

	// git update-ref -d - can delete refs including HEAD
	{regex: regexp.MustCompile(`(?i)\bgit\s+update-ref\s+.*-d\b`), name: "git update-ref -d"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+update-ref\s+.*--delete\b`), name: "git update-ref --delete"},

	// === Discard Changes ===

	// git switch - all forms blocked, user must switch branches manually
	{regex: regexp.MustCompile(`(?i)\bgit\s+switch\b`), name: "git switch (user must switch branches manually)"},

	// git cherry-pick --abort - discards in-progress cherry-pick work
	{regex: regexp.MustCompile(`(?i)\bgit\s+cherry-pick\s+.*--abort\b`), name: "git cherry-pick --abort"},

	// git merge --abort - discards merge in progress
	{regex: regexp.MustCompile(`(?i)\bgit\s+merge\s+.*--abort\b`), name: "git merge --abort"},

	// git worktree remove --force - force removes worktree
	{regex: regexp.MustCompile(`(?i)\bgit\s+worktree\s+remove\s+.*--force\b`), name: "git worktree remove --force"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+worktree\s+remove\s+.*-f\b`), name: "git worktree remove -f"},

	// git submodule deinit --force - removes submodule working directory
	{regex: regexp.MustCompile(`(?i)\bgit\s+submodule\s+deinit\s+.*--force\b`), name: "git submodule deinit --force"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+submodule\s+deinit\s+.*-f\b`), name: "git submodule deinit -f"},

	// === Non-Git Repository Destruction ===

	// rm -rf .git - destroys the entire repository
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+.*\.git\b`), name: "rm -rf .git"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*f[a-zA-Z]*r[a-zA-Z]*\s+.*\.git\b`), name: "rm -fr .git"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*\.git/`), name: "rm .git/ (repository file deletion)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*\.git/index\b`), name: "rm .git/index (staging area corruption)"},

	// Lock file deletion - can corrupt in-progress git operations
	{regex: regexp.MustCompile(`(?i)\brm\s+.*\.git/index\.lock\b`), name: "rm .git/index.lock (can corrupt staging)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*\.git/.*\.lock\b`), name: "rm .git/*.lock (can corrupt git operations)"},

	// === Filesystem Destruction ===

	// rm -rf on critical paths
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+/\s*$`), name: "rm -rf / (system wipe)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+/\*`), name: "rm -rf /* (system wipe)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+~/?`), name: "rm -rf ~ (home directory wipe)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+\$HOME`), name: "rm -rf $HOME (home directory wipe)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+\.\./?`), name: "rm -rf .. (parent directory wipe)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+\.\s*$`), name: "rm -rf . (current directory wipe)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+\*\s*$`), name: "rm -rf * (current directory wipe)"},

	// Critical system directories
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+/(etc|var|usr|bin|sbin|lib|boot|root|home)\b`), name: "rm -rf system directory"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+/Applications\b`), name: "rm -rf /Applications (macOS apps)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+/System\b`), name: "rm -rf /System (macOS system)"},
	{regex: regexp.MustCompile(`(?i)\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+/Library\b`), name: "rm -rf /Library (macOS library)"},

	// === Disk/Partition Destruction ===

	// dd to disk devices - can wipe entire drives
	{regex: regexp.MustCompile(`(?i)\bdd\s+.*of\s*=\s*/dev/(sd|hd|nvme|vd|xvd|disk)`), name: "dd to disk device (disk wipe)"},
	{regex: regexp.MustCompile(`(?i)\bdd\s+.*of\s*=\s*/dev/null`), name: "dd to /dev/null", exclude: regexp.MustCompile(`.*`)}, // Allow this one actually
	{regex: regexp.MustCompile(`(?i)>\s*/dev/(sd|hd|nvme|vd|xvd|disk)`), name: "redirect to disk device (disk wipe)"},

	// Filesystem formatting
	{regex: regexp.MustCompile(`(?i)\bmkfs\b`), name: "mkfs (filesystem format)"},
	{regex: regexp.MustCompile(`(?i)\bmkswap\b`), name: "mkswap (swap format)"},
	{regex: regexp.MustCompile(`(?i)\bfdisk\b`), name: "fdisk (partition table modification)"},
	{regex: regexp.MustCompile(`(?i)\bparted\b`), name: "parted (partition modification)"},
	{regex: regexp.MustCompile(`(?i)\bgdisk\b`), name: "gdisk (GPT partition modification)"},
	{regex: regexp.MustCompile(`(?i)\bdiskutil\s+(eraseDisk|eraseVolume|partitionDisk|secureErase)`), name: "diskutil destructive operation"},

	// === System Commands ===

	// System shutdown/reboot
	{regex: regexp.MustCompile(`(?i)\bshutdown\b`), name: "shutdown"},
	{regex: regexp.MustCompile(`(?i)\breboot\b`), name: "reboot"},
	{regex: regexp.MustCompile(`(?i)\bhalt\b`), name: "halt"},
	{regex: regexp.MustCompile(`(?i)\bpoweroff\b`), name: "poweroff"},
	{regex: regexp.MustCompile(`(?i)\binit\s+[0-6]\b`), name: "init runlevel change"},
	{regex: regexp.MustCompile(`(?i)\bsystemctl\s+(halt|poweroff|reboot|suspend|hibernate)`), name: "systemctl power command"},

	// Process destruction
	{regex: regexp.MustCompile(`(?i)\bkill\s+.*-9\s+(-1|1)\b`), name: "kill -9 -1 (kill all processes)"},
	{regex: regexp.MustCompile(`(?i)\bkillall\s+-9\b`), name: "killall -9"},
	{regex: regexp.MustCompile(`(?i)\bpkill\s+-9\b`), name: "pkill -9"},

	// Fork bomb patterns
	{regex: regexp.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;`), name: "fork bomb"},
	{regex: regexp.MustCompile(`(?i)\bforkbomb\b`), name: "fork bomb"},

	// === Permission Destruction ===

	// Recursive chmod on system paths
	{regex: regexp.MustCompile(`(?i)\bchmod\s+.*-[rR].*\s+/\s*$`), name: "chmod -R / (system permission change)"},
	{regex: regexp.MustCompile(`(?i)\bchmod\s+.*-[rR].*\s+/(etc|var|usr|bin|sbin|lib|boot|root|home)\b`), name: "chmod -R system directory"},
	{regex: regexp.MustCompile(`(?i)\bchmod\s+.*000\s`), name: "chmod 000 (remove all permissions)"},
	{regex: regexp.MustCompile(`(?i)\bchmod\s+.*777\s+/`), name: "chmod 777 on system path"},

	// Recursive chown on system paths
	{regex: regexp.MustCompile(`(?i)\bchown\s+.*-[rR].*\s+/\s*$`), name: "chown -R / (system ownership change)"},
	{regex: regexp.MustCompile(`(?i)\bchown\s+.*-[rR].*\s+/(etc|var|usr|bin|sbin|lib|boot|root|home)\b`), name: "chown -R system directory"},

	// === Database Destruction ===

	// SQL destructive commands
	{regex: regexp.MustCompile(`(?i)\bDROP\s+(DATABASE|SCHEMA)\b`), name: "DROP DATABASE/SCHEMA"},
	{regex: regexp.MustCompile(`(?i)\bDROP\s+TABLE\b`), name: "DROP TABLE"},
	{regex: regexp.MustCompile(`(?i)\bTRUNCATE\s+(TABLE\s+)?\w`), name: "TRUNCATE TABLE"},
	{regex: regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+\w+\s*;`), name: "DELETE FROM without WHERE clause"},
	{regex: regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+\w+\s*$`), name: "DELETE FROM without WHERE clause"},

	// MongoDB destructive commands
	{regex: regexp.MustCompile(`(?i)\.drop\s*\(\s*\)`), name: "MongoDB .drop()"},
	{regex: regexp.MustCompile(`(?i)\.dropDatabase\s*\(\s*\)`), name: "MongoDB .dropDatabase()"},
	{regex: regexp.MustCompile(`(?i)\.deleteMany\s*\(\s*\{\s*\}\s*\)`), name: "MongoDB .deleteMany({}) (delete all)"},

	// Redis destructive commands
	{regex: regexp.MustCompile(`(?i)\bFLUSHALL\b`), name: "Redis FLUSHALL"},
	{regex: regexp.MustCompile(`(?i)\bFLUSHDB\b`), name: "Redis FLUSHDB"},

	// === Docker/Container Destruction ===

	// Docker system-wide destruction
	{regex: regexp.MustCompile(`(?i)\bdocker\s+system\s+prune\s+.*-a`), name: "docker system prune -a (remove all)"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+system\s+prune\s+.*--all`), name: "docker system prune --all"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+(rm|rmi|volume\s+rm|network\s+rm)\s+.*-f`), name: "docker force remove"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+(rm|rmi)\s+.*\$\(docker\s+(ps|images)`), name: "docker remove all containers/images"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+container\s+prune\s+-f`), name: "docker container prune -f"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+image\s+prune\s+-a`), name: "docker image prune -a"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+volume\s+prune\s+-f`), name: "docker volume prune -f"},

	// Docker Compose destruction
	{regex: regexp.MustCompile(`(?i)\bdocker-compose\s+down\s+.*-v`), name: "docker-compose down -v (removes volumes)"},
	{regex: regexp.MustCompile(`(?i)\bdocker\s+compose\s+down\s+.*-v`), name: "docker compose down -v (removes volumes)"},

	// === Kubernetes Destruction ===

	// Namespace/cluster-wide deletion
	{regex: regexp.MustCompile(`(?i)\bkubectl\s+delete\s+(namespace|ns)\b`), name: "kubectl delete namespace"},
	{regex: regexp.MustCompile(`(?i)\bkubectl\s+delete\s+.*--all\s+--all-namespaces`), name: "kubectl delete all in all namespaces"},
	{regex: regexp.MustCompile(`(?i)\bkubectl\s+delete\s+.*-A\s+--all`), name: "kubectl delete all cluster-wide"},
	{regex: regexp.MustCompile(`(?i)\bkubectl\s+delete\s+all\s+--all`), name: "kubectl delete all --all"},

	// Helm destructive commands
	{regex: regexp.MustCompile(`(?i)\bhelm\s+uninstall\s+.*--no-hooks`), name: "helm uninstall --no-hooks"},

	// === Cloud/Infrastructure Destruction ===

	// Terraform destruction
	{regex: regexp.MustCompile(`(?i)\bterraform\s+destroy\b`), name: "terraform destroy"},
	{regex: regexp.MustCompile(`(?i)\bterraform\s+apply\s+.*-destroy`), name: "terraform apply -destroy"},
	{regex: regexp.MustCompile(`(?i)\btofu\s+destroy\b`), name: "tofu destroy"},
	{regex: regexp.MustCompile(`(?i)\bpulumi\s+destroy\b`), name: "pulumi destroy"},

	// AWS destructive commands
	{regex: regexp.MustCompile(`(?i)\baws\s+s3\s+rm\s+.*--recursive`), name: "aws s3 rm --recursive"},
	{regex: regexp.MustCompile(`(?i)\baws\s+s3\s+rb\s+.*--force`), name: "aws s3 rb --force (bucket deletion)"},
	{regex: regexp.MustCompile(`(?i)\baws\s+ec2\s+terminate-instances\b`), name: "aws ec2 terminate-instances"},
	{regex: regexp.MustCompile(`(?i)\baws\s+rds\s+delete-db-instance\b`), name: "aws rds delete-db-instance"},
	{regex: regexp.MustCompile(`(?i)\baws\s+rds\s+delete-db-cluster\b`), name: "aws rds delete-db-cluster"},
	{regex: regexp.MustCompile(`(?i)\baws\s+cloudformation\s+delete-stack\b`), name: "aws cloudformation delete-stack"},

	// GCP destructive commands
	{regex: regexp.MustCompile(`(?i)\bgcloud\s+.*\s+delete\b`), name: "gcloud delete command"},
	{regex: regexp.MustCompile(`(?i)\bgsutil\s+rm\s+.*-r`), name: "gsutil rm -r (recursive delete)"},

	// Azure destructive commands
	{regex: regexp.MustCompile(`(?i)\baz\s+group\s+delete\b`), name: "az group delete (resource group)"},
	{regex: regexp.MustCompile(`(?i)\baz\s+.*\s+delete\b`), name: "az delete command"},

	// === Arbitrary Code Execution ===

	// Piping to shell - dangerous remote code execution
	{regex: regexp.MustCompile(`(?i)\bcurl\s+.*\|\s*(ba)?sh\b`), name: "curl | sh (remote code execution)"},
	{regex: regexp.MustCompile(`(?i)\bwget\s+.*\|\s*(ba)?sh\b`), name: "wget | sh (remote code execution)"},
	{regex: regexp.MustCompile(`(?i)\bcurl\s+.*\|\s*sudo\b`), name: "curl | sudo (remote code as root)"},
	{regex: regexp.MustCompile(`(?i)\bwget\s+.*\|\s*sudo\b`), name: "wget | sudo (remote code as root)"},
	{regex: regexp.MustCompile(`(?i)\bcurl\s+.*\|\s*bash\s+-`), name: "curl | bash - (remote code execution)"},
	{regex: regexp.MustCompile(`(?i)\bwget\s+.*-O\s*-\s*\|\s*(ba)?sh`), name: "wget -O - | sh (remote code execution)"},

	// eval with external input
	{regex: regexp.MustCompile(`(?i)\beval\s+.*\$\(`), name: "eval with command substitution"},
	{regex: regexp.MustCompile(`(?i)\beval\s+.*\bcurl\b`), name: "eval with curl"},
	{regex: regexp.MustCompile(`(?i)\beval\s+.*\bwget\b`), name: "eval with wget"},

	// === Privilege Escalation ===

	// sudo - all sudo commands require user approval
	{regex: regexp.MustCompile(`(?i)\bsudo\b`), name: "sudo (requires user approval)"},

	// === Git Plumbing Bypasses ===
	// Low-level plumbing commands that bypass the high-level protections above.

	// git read-tree - resets/overwrites the index (staging area) to any tree-ish
	// Bypasses: git reset, git restore --staged
	{regex: regexp.MustCompile(`(?i)\bgit\s+read-tree\b`), name: "git read-tree (index manipulation bypass)"},

	// git update-index - directly manipulates the index entries
	// Can unstage files, hide changes (--assume-unchanged, --skip-worktree), or remove entries
	// Bypasses: git reset, git restore --staged, git add
	{regex: regexp.MustCompile(`(?i)\bgit\s+update-index\b`), name: "git update-index (direct index manipulation)"},

	// git symbolic-ref - changes what HEAD points to, effectively switching branches
	// Bypasses: git checkout, git switch
	// Allow read-only usage: git symbolic-ref [--short|-q] HEAD
	// Block write usage: git symbolic-ref HEAD refs/heads/main (2 non-flag args)
	{regex: regexp.MustCompile(`(?i)\bgit\s+symbolic-ref\s+(-\S+\s+)*[^-\s]\S*\s+[^-\s]`), name: "git symbolic-ref (HEAD manipulation bypass)"},

	// git checkout-index - overwrites working tree files from the index
	// Bypasses: git checkout -- <file>, git restore <file>
	{regex: regexp.MustCompile(`(?i)\bgit\s+checkout-index\b`), name: "git checkout-index (working tree overwrite)"},

	// git replace - replaces any git object with another, can silently rewrite history
	{regex: regexp.MustCompile(`(?i)\bgit\s+replace\b`), name: "git replace (object replacement)"},

	// === Convex Typecheck Bypass ===

	// Convex commands with typecheck disabled - prevents deploying unchecked code
	{regex: regexp.MustCompile(`(?i)\b(npx\s+)?convex\s+(dev|deploy)\s+.*--typecheck\s*=\s*disable`), name: "convex with --typecheck=disable (unsafe deployment)"},
	{regex: regexp.MustCompile(`(?i)\b(npx\s+)?convex\s+(dev|deploy)\s+.*--typecheck\s+disable`), name: "convex with --typecheck disable (unsafe deployment)"},
}

// hookBypassPatterns contains patterns that attempt to skip pre-commit hooks or checks.
var hookBypassPatterns = []pattern{
	// Environment variables that skip checks
	{regex: regexp.MustCompile(`(?i)\bSKIP_PRECOMMIT_CHECKS\s*=`), name: "SKIP_PRECOMMIT_CHECKS"},
	{regex: regexp.MustCompile(`(?i)\bSKIP_PRE_COMMIT\s*=`), name: "SKIP_PRE_COMMIT"},
	{regex: regexp.MustCompile(`(?i)\bSKIP_HOOKS?\s*=`), name: "SKIP_HOOK(S)"},
	{regex: regexp.MustCompile(`(?i)\bSKIP_TESTS\s*=`), name: "SKIP_TESTS"},
	{regex: regexp.MustCompile(`(?i)\bHUSKY\s*=\s*0\b`), name: "HUSKY=0"},
	{regex: regexp.MustCompile(`(?i)\bHUSKY_SKIP_HOOKS\s*=`), name: "HUSKY_SKIP_HOOKS"},
	{regex: regexp.MustCompile(`(?i)\bPRE_COMMIT_ALLOW_NO_CONFIG\s*=`), name: "PRE_COMMIT_ALLOW_NO_CONFIG"},

	// Git flags that skip hooks (use .* after git to handle global flags like -C)
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*\bcommit\s+.*--no-verify\b`), name: "git commit --no-verify"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*\bcommit\s+.*-n\b`), name: "git commit -n"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*\bpush\s+.*--no-verify\b`), name: "git push --no-verify"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*\bmerge\s+.*--no-verify\b`), name: "git merge --no-verify"},
}

// gitCommandRegex detects any git command invocation and extracts the subcommand.
// Handles global flags: simple flags (--no-pager), flags with = args (--git-dir=/path),
// and flags with separate args (-C /path, -c key=val).
var gitCommandRegex = regexp.MustCompile(`(?i)\bgit\s+(?:(?:-[Cc]\s+\S+|--?\S+)\s+)*(\S+)`)

// allowedGitSubcommands is the whitelist of git subcommands Claude is permitted to run.
// Everything NOT on this list is blocked. This is the primary security mechanism for git.
var allowedGitSubcommands = map[string]bool{
	// Core workflow (the 4 the user wants)
	"add":    true,
	"commit": true,
	"push":   true,
	"pull":   true,

	// Read-only / informational
	"status":       true,
	"diff":         true,
	"log":          true,
	"show":         true,
	"branch":       true, // listing; -D is caught by blacklist above
	"describe":     true,
	"blame":        true,
	"shortlog":     true,
	"reflog":       true, // expire/delete caught by blacklist above
	"remote":       true, // -v listing; modifying subcommands caught below
	"tag":          true, // listing; create/delete caught below
	"fetch":        true,
	"grep":         true,
	"name-rev":     true,
	"verify-commit": true,
	"verify-tag":   true,

	// Plumbing read-only
	"ls-files":     true,
	"ls-tree":      true,
	"ls-remote":    true,
	"rev-parse":    true,
	"rev-list":     true,
	"cat-file":     true,
	"for-each-ref": true,
	"merge-base":   true,
	"diff-tree":    true,
	"diff-files":   true,
	"diff-index":   true,
	"hash-object":  true, // read-only unless -w, but low risk
	"var":          true,
}

// gitModifyingSubcommands are subcommands that are allowed in the whitelist above but
// have specific subcommands/flags that modify state and should be blocked.
var gitModifyingPatterns = []pattern{
	// git remote - allow listing (git remote, git remote -v) but block modifications
	{regex: regexp.MustCompile(`(?i)\bgit\s+remote\s+(add|remove|rm|rename|set-url|set-head|prune)\b`), name: "git remote modification (only listing allowed)"},

	// git tag - allow listing (git tag, git tag -l) but block create/delete
	{regex: regexp.MustCompile(`(?i)\bgit\s+tag\s+(-d|--delete)\b`), name: "git tag delete"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+tag\s+(-a|--annotate|-s|--sign|-f|--force)\b`), name: "git tag create"},
	// git tag <name> (creating a tag - has a non-flag argument)
	{regex: regexp.MustCompile(`(?i)\bgit\s+tag\s+[^-]\S*\s`), name: "git tag create"},

	// git config - all modifications blocked
	{regex: regexp.MustCompile(`(?i)\bgit\s+config\b`), name: "git config (user must modify config manually)"},
}

func main() {
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "BLOCKED: failed to parse hook input: %v\n\nBlocking by default when input cannot be parsed.\n", err)
		os.Exit(2)
	}

	cmd := input.ToolInput.Command
	if cmd == "" {
		cmd = input.Command // fallback for flat format
	}
	if cmd == "" {
		os.Exit(0)
	}

	// Check for destructive commands (specific blacklist with clear error messages)
	for _, p := range destructivePatterns {
		if p.regex.MatchString(cmd) {
			// Skip if exclude pattern matches (e.g., git rm --cached is allowed)
			if p.exclude != nil && p.exclude.MatchString(cmd) {
				continue
			}
			fmt.Fprintf(os.Stderr, `BLOCKED: %s

This command is blocked because it can cause catastrophic data loss or system damage.

Blocked command: %s

If you need to run this command, ask the user to do it manually.
`, p.name, cmd)
			os.Exit(2)
		}
	}

	// Check for hook bypass attempts
	for _, p := range hookBypassPatterns {
		if p.regex.MatchString(cmd) {
			// Skip if exclude pattern matches
			if p.exclude != nil && p.exclude.MatchString(cmd) {
				continue
			}
			fmt.Fprintf(os.Stderr, `BLOCKED: %s

Skipping pre-commit hooks or checks is not allowed.

Blocked command: %s

Pre-commit hooks exist to maintain code quality. If checks are failing:
1. Fix the underlying issues (lint errors, type errors, test failures)
2. If the issues are unrelated to your changes, ask the user to run the commit manually

Do not bypass hooks - ask the user to do it if absolutely necessary.
`, p.name, cmd)
			os.Exit(2)
		}
	}

	// Git whitelist check: if the command contains a git invocation,
	// verify the subcommand is in the allowed list. This catches any
	// plumbing commands or obscure subcommands not in the blacklist above.
	if matches := gitCommandRegex.FindStringSubmatch(cmd); matches != nil {
		subcommand := strings.ToLower(matches[1])

		// Check if the subcommand is whitelisted
		if !allowedGitSubcommands[subcommand] {
			fmt.Fprintf(os.Stderr, `BLOCKED: git %s (not in allowed git commands)

Only the following git commands are permitted: add, commit, push, pull,
status, diff, log, show, branch, fetch, blame, describe, shortlog,
reflog, remote (list), tag (list), grep, and read-only plumbing commands.

Blocked command: %s

If you need to run this command, ask the user to do it manually.
`, subcommand, cmd)
			os.Exit(2)
		}

		// Even for whitelisted subcommands, check for modifying patterns
		for _, p := range gitModifyingPatterns {
			if p.regex.MatchString(cmd) {
				if p.exclude != nil && p.exclude.MatchString(cmd) {
					continue
				}
				fmt.Fprintf(os.Stderr, `BLOCKED: %s

This git subcommand modification is not allowed.

Blocked command: %s

If you need to run this command, ask the user to do it manually.
`, p.name, cmd)
				os.Exit(2)
			}
		}
	}

	os.Exit(0)
}

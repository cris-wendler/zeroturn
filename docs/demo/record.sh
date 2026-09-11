#!/bin/sh
# Produces the output shown in the README from the real executable.
#
# Everything runs in a throwaway directory with its own Git configuration
# and its own ZeroTurn state, so nothing on the machine running it changes.
# The session values come from fixtures/claude/statusline-full.json and are
# sample data, not a real account.
#
# Approving the sample project's validation commands needs a terminal, so
# the script answers that one prompt with expect, which ships with macOS.
#
# Usage: docs/demo/record.sh [path to zeroturn]
set -eu

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
zt=${1:-zeroturn}
demo=$(mktemp -d)
trap 'rm -rf "$demo"' EXIT

export ZEROTURN_STATE_DIR="$demo/state"
export GIT_CONFIG_GLOBAL="$demo/gitconfig"
export GIT_CONFIG_NOSYSTEM=1
export NO_COLOR=1
printf '[user]\n\tname = Demo\n\temail = demo@example.invalid\n[init]\n\tdefaultBranch = main\n' >"$GIT_CONFIG_GLOBAL"

git init --quiet --bare "$demo/remote.git"
git clone --quiet "$demo/remote.git" "$demo/app" 2>/dev/null
cd "$demo/app"
git remote set-url origin ../remote.git

printf 'module example.com/app\n\ngo 1.17\n' >go.mod
printf 'package app\n\nfunc Add(a, b int) int { return a + b }\n' >app.go
printf 'package app\n\nimport "testing"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal("wrong")\n\t}\n}\n' >app_test.go
git add -A
git commit --quiet --message "initial"
git push --quiet --set-upstream origin main 2>/dev/null
git checkout --quiet -b docs-update
git push --quiet --set-upstream origin docs-update 2>/dev/null

"$zt" init --yes >/dev/null
"$zt" policy set guard.mode confirm >/dev/null
expect -c "set timeout 60; spawn $zt verify --approve; expect {Execute these commands}; send y\r; expect eof" >/dev/null

session() {
	sed -e "s#/fixture/repo#$demo/app#g" \
		-e 's#"session_id": "fixture-full"#"session_id": "demo"#' \
		-e 's#"used_percentage": 76#"used_percentage": 82#' \
		"$repo_root/fixtures/claude/statusline-full.json"
}

agent_start() {
	printf '{"session_id":"demo","hook_event_name":"SubagentStart","cwd":"%s","agent_id":"%s","agent_type":"Explore"}' "$demo/app" "$1" |
		"$zt" event --harness claude --event SubagentStart
}

propose_subagent() {
	printf '{"session_id":"demo","hook_event_name":"PreToolUse","cwd":"%s","tool_name":"Agent","tool_input":{"prompt":"sample"}}' "$demo/app" |
		"$zt" event --harness claude --event PreToolUse
}

session | "$zt" status --stdin --harness claude >/dev/null
agent_start a1
agent_start a2

echo '$ zeroturn status --stdin --harness claude < session.json'
session | "$zt" status --stdin --harness claude
echo
echo '# the harness proposes another subagent'
propose_subagent
echo
echo '$ zeroturn verify'
"$zt" verify || true
echo
printf '# Release notes\n\nFirst draft.\n' >NOTES.md
echo '$ zeroturn ship --message "docs: add release notes" --files NOTES.md --dry-run'
"$zt" ship --message "docs: add release notes" --files NOTES.md --dry-run || true

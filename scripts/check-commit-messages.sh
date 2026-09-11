#!/bin/sh
# Fails when any commit message credits a coding tool, for example with a
# Co-Authored-By line. Human co-authors are allowed. The project's commits
# carry only the names of the people who wrote them.
#
# Usage: scripts/check-commit-messages.sh [revision range]
set -u
range=${1:-HEAD}
pattern='^co-authored-by:.*(claude|anthropic|copilot|openai|chatgpt|gemini|cursor|codeium|aider)|noreply@anthropic\.com|generated (with|by) \[?(claude|copilot|an ai)'
found=$(git log --format='%h %B' "$range" | grep -i -n -E "$pattern")
if [ -n "$found" ]; then
	echo "$found"
	echo
	echo "A commit message credits a coding tool."
	echo "Rewrite the commit message without that line, then push again."
	exit 1
fi
echo "commit messages: no coding tool attribution"

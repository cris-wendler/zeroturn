#!/bin/sh
# Checks tracked files against the project's writing rules.
#
# The license texts are official documents and are not checked. Test files
# and fixtures may contain sample paths on purpose, so the private path
# check skips them. This script and scripts/check-commit-messages.sh name
# the phrases they look for, so both are skipped.
set -u

cd "$(dirname "$0")/.." || exit 2

em=$(printf '\342\200\224')
en=$(printf '\342\200\223')
status=0

report() {
	# $1 category, remaining arguments are grep output lines
	category=$1
	shift
	for line in "$@"; do
		printf '%s  %s\n' "$line" "$category"
		status=1
	done
}

check() {
	# $1 category, $2 grep flags, $3 pattern, $4 file
	out=$(grep -n $2 -e "$3" "$4" 2>/dev/null | sed "s#^#$4:#" | cut -c1-160)
	if [ -n "$out" ]; then
		old_ifs=$IFS
		IFS='
'
		# shellcheck disable=SC2086
		report "$1" $out
		IFS=$old_ifs
	fi
}

promotional='AI[- ]powered|agentic|revolutionary|next[- ]generation|game[- ]changing|intelligent automation|seamless|supercharge|transform your workflow|future of development|cutting[- ]edge|blazing[- ]fast|effortless|\bmagic\b|autonomous engineering|leverage|harness the power|redefine|\bdisrupt(s|ing)?\b|best[- ]in[- ]class|enterprise[- ]grade|battle[- ]tested|one[- ]stop solution'
# unlock is a promotional word in prose and an ordinary identifier in Go.
promotional_prose="$promotional|unlock"
filler="in today's rapidly changing world|whether you are a beginner or an expert|say goodbye to|look no further|at its core|it is important to note|this powerful tool|this comprehensive solution|the possibilities are endless|welcome to the future"
attribution='generated (by|with)|created with (claude|copilot|chatgpt|an ai)|written by an ai|co-authored-by:'
savings='zero tokens|save[sd]? tokens|token savings|fewer tokens|reduc(e|es|ed) (your )?(token|usage|cost)s? by|guaranteed savings'
prompt_text='build a professional open source command line project|do not begin by scaffolding code|ask one concise question only if'
placeholder='github\.com/OWNER|OWNER/tap|your-org/|example-owner'
private_path='/Users/[A-Za-z]|/home/[a-z][a-z0-9_-]*/|C:\\\\Users\\\\'

# Files not yet committed are checked too, so a problem is found before
# it reaches a commit.
files=$(git ls-files --cached --others --exclude-standard | grep -v -E '^(LICENSE|COPYING)$' | grep -v -E '\.(svg|png|gif|jpg)$' | grep -v -E '^scripts/(lint-copy|check-commit-messages)\.sh$')

for f in $files; do
	[ -f "$f" ] || continue
	check "em dash" "-F" "$em" "$f"
	check "en dash" "-F" "$en" "$f"
	case "$f" in
	*.go) check "promotional phrase" "-i -E" "$promotional" "$f" ;;
	*) check "promotional phrase" "-i -E" "$promotional_prose" "$f" ;;
	esac
	check "generic filler" "-i -E" "$filler" "$f"
	check "generated attribution" "-i -E" "$attribution" "$f"
	check "savings claim" "-i -E" "$savings" "$f"
	check "prompt text" "-i -E" "$prompt_text" "$f"
	check "placeholder owner" "-E" "$placeholder" "$f"
	case "$f" in
	*_test.go | fixtures/*) ;;
	*) check "private path" "-E" "$private_path" "$f" ;;
	esac
done

if [ "$status" -ne 0 ]; then
	echo
	echo "lint-copy found text that breaks the writing rules in CONTRIBUTING.md."
	echo "Rewrite the lines above, then run scripts/lint-copy.sh again."
	exit 1
fi
echo "lint-copy: no problems found"

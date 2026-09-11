#!/bin/sh
# A worked example of a ZeroTurn adapter.
#
# An adapter translates one harness event into the normalized event
# ZeroTurn understands, sends it on standard input, and passes any
# decision back to the harness. This example reads a made up harness
# payload and handles the subagent gate.
#
# Copy it, change read_field and the event names, and check the result
# with:
#   go run ./conformance/validate schemas/normalized-event.schema.json event.json
#
# Usage: adapter.sh <harness event name> < payload.json
set -eu

zeroturn=${ZEROTURN:-zeroturn}
event_name=${1:?usage: adapter.sh <harness event name> < payload.json}
payload=$(cat)

# read_field pulls one value out of the harness payload. This example uses
# jq for clarity. An adapter written in the harness's own language should
# use that language's JSON support instead of starting another process.
read_field() {
	printf '%s' "$payload" | jq -r "$1 // empty"
}

case "$event_name" in
toolCallProposed) type="subagent.pre" ;;
subagentStarted) type="subagent.start" ;;
subagentFinished) type="subagent.stop" ;;
turnEnded) type="session.stop" ;;
sessionEnded) type="session.end" ;;
sessionStatus) type="status" ;;
*)
	# An event ZeroTurn does not model is not an error. Ignore it.
	exit 0
	;;
esac

# Only the gate acts on a proposed subagent. Other tools are not gated.
if [ "$type" = "subagent.pre" ] && [ "$(read_field .tool)" != "subagent" ]; then
	exit 0
fi

# Measurements the harness did not report are left out, never sent as
# zero. Nothing from a prompt, a response, or a file is ever included.
event=$(
	printf '%s' "$payload" | jq -c \
		--arg type "$type" \
		--arg harness "example" \
		'{
			contract: "zeroturn.event/1",
			harness: $harness,
			type: $type,
			sessionId: .session.id,
			cwd: .session.workingDirectory
		}
		+ (if .session.version then {harnessVersion: .session.version} else {} end)
		+ (if .usage.contextPercent then {contextPercent: .usage.contextPercent} else {} end)
		+ (if .usage.fiveHourPercent then {fiveHourPercent: .usage.fiveHourPercent} else {} end)
		+ (if .session.durationMs then {durationMs: .session.durationMs} else {} end)
		+ (if .subagent.id then {agentId: .subagent.id} else {} end)
		+ (if .subagent.kind then {agentType: .subagent.kind} else {} end)
		+ (if .backgroundTasks then {backgroundTasks: (.backgroundTasks | length)} else {} end)'
)

decision=$(printf '%s' "$event" | "$zeroturn" event --harness normalized 2>/dev/null || true)

# No output means allow. ZeroTurn stays silent so it never overrides the
# permission rules the user already set in the harness.
if [ -z "$decision" ]; then
	exit 0
fi

# Translate the decision into whatever this harness expects. Never turn an
# ask into a deny, and never claim to block when the harness cannot.
verdict=$(printf '%s' "$decision" | jq -r .hookSpecificOutput.permissionDecision)
reason=$(printf '%s' "$decision" | jq -r .hookSpecificOutput.permissionDecisionReason)

case "$verdict" in
ask) printf '{"action":"confirm","message":%s}\n' "$(printf '%s' "$reason" | jq -R .)" ;;
deny) printf '{"action":"reject","message":%s}\n' "$(printf '%s' "$reason" | jq -R .)" ;;
*) exit 0 ;;
esac

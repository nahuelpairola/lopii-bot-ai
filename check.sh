#!/usr/bin/env bash
#
# The one command that proves a change works:  bash check.sh
#
# Runs build, vet, errcheck and the default test suite. Replaces the old
# `make lint` — there is no `make` on the Windows/Git Bash box this repo is
# developed on, so that target was documented but unrunnable.
#
# The default suite has ONE expected failure,
# TestEveryConfigFile_HasNoSameTurnModelCollision: red by decision since
# 2026-08-17 (see internal/config/config_test.go and AGENTS.md). This script
# excuses that one by name and nothing else, so a green exit still means
# something. Any other FAIL exits 1.
#
# Needs Postgres only for the tagged suites (integration/query_eval), which
# this script does not run — see the build-tag table in AGENTS.md.

set -u
cd "$(dirname "$0")" || exit 1

known_red='TestEveryConfigFile_HasNoSameTurnModelCollision'
status=0

step() {
	echo "== $* =="
	"$@" || status=1
}

step go build ./...
step go vet ./...

# errcheck is INFORMATIONAL, not a gate. The tree carries ~215 pre-existing
# findings (~38 outside tests, nearly all unchecked bot.SendMessage). Failing
# the whole check on debt a change did not introduce teaches everyone to ignore
# the exit code. Read the diff between your own lines and the noise.
if command -v errcheck >/dev/null 2>&1; then
	echo "== errcheck ./... (informational — pre-existing debt, not a gate) =="
	ec=$(errcheck ./... 2>&1)
	if [ -z "$ec" ]; then
		echo "   clean."
	else
		echo "$ec" | grep -v '_test.go'
		echo "   $(echo "$ec" | wc -l) findings total; check whether any are yours."
	fi
else
	echo "== errcheck: NOT INSTALLED — skipped =="
	echo "   go install github.com/kisielk/errcheck@latest"
fi

echo "== go test ./... =="
out=$(go test ./... 2>&1)
echo "$out"

if echo "$out" | grep -q '^FAIL'; then
	unexpected=$(echo "$out" | grep -- '--- FAIL' | grep -v "$known_red")
	if [ -n "$unexpected" ]; then
		echo ""
		echo "REAL failures (not the known red):"
		echo "$unexpected"
		status=1
	else
		echo ""
		echo "Only the known red: $known_red"
		echo "Red by decision — do NOT swap models to make it green (AGENTS.md)."
	fi
fi

echo ""
[ "$status" -eq 0 ] && echo "check: OK" || echo "check: FAILED"
exit "$status"

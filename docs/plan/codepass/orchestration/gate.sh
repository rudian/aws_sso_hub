#!/bin/bash
# The full gate on the main checkout (branch codepass). One summary line per step; logs go to $TMPDIR/codepass-gate/.
# Usage: bash docs/plan/codepass/orchestration/gate.sh
cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)" || exit 2
S="${TMPDIR:-/tmp}/codepass-gate"; mkdir -p "$S"
go vet ./... >"$S/vet.txt" 2>&1; echo "vet=$?"
go test ./... >"$S/test.txt" 2>&1; echo "test=$?"; grep -E '^(FAIL|---)' "$S/test.txt" | head -20
GOOS=windows GOARCH=amd64 go build ./... >"$S/win.txt" 2>&1; echo "winbuild=$?"
(cd frontend && npm run typecheck >"$S/tc.txt" 2>&1; echo "typecheck=$?"
 npm test >"$S/fe.txt" 2>&1; echo "fetest=$?"; grep -E 'Test Files|Tests |Errors' "$S/fe.txt" | tail -3
 npm run build >"$S/build.txt" 2>&1; echo "build=$?"
 npm run check:dist >"$S/cd.txt" 2>&1; echo "checkdist=$?")
CODEPASS_SCRUB_LIST="$HOME/.codepass-dev/scrub-list.txt" bash scripts/scrub-check.sh --tracked >"$S/scrub.txt" 2>&1; echo "scrub=$?"
echo "status-lines=$(git status --short | wc -l | tr -d ' ')"; git ls-files frontend/dist | head -1

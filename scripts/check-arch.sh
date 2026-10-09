#!/usr/bin/env bash
set -euo pipefail
# =============================================================================
# Architecture constraint checker (run by `task arch-check`, pre-commit and CI)
# =============================================================================
#
# Dependencies must point inward:
#
#   services/*  →  internal/{http,middleware,wire}  →  internal/domains
#                                                        ↓ (interfaces only)
#   internal/storage/mongo/repository  ──implements──▶  internal/storage/interfaces
#   internal/model  ←  everyone            pkg/*  ←  everyone (never the reverse)
#
# Each rule: a directory, an import pattern that is forbidden inside it, and why.
# Test files are included on purpose: tests must respect the layering too,
# except for the *_test packages listed in ALLOW_TEST_PATTERN.
# =============================================================================

MODULE="github.com/aspirit/xsocial"
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
VIOLATIONS=0

# forbid <dir> <regex on import path> <message>
forbid() {
    local dir="$1" pattern="$2" message="$3"
    [ -d "$dir" ] || return 0
    local hits
    hits=$(grep -rnE --include='*.go' "\"${MODULE}/(${pattern})" "$dir" 2>/dev/null || true)
    if [ -n "$hits" ]; then
        echo -e "${RED}❌ ${message}${NC}"
        echo "$hits" | sed "s/^/   ${YELLOW}/;s/\$/${NC}/" | while IFS= read -r l; do echo -e "$l"; done
        VIOLATIONS=$((VIOLATIONS + 1))
    fi
}

echo "🔍 Checking architecture constraints..."

forbid pkg 'internal|services' \
    "pkg/* is reusable library code and MUST NOT import internal/* or services/*"

forbid internal/model 'internal/(domains|storage|http|middleware|wire)|services' \
    "internal/model holds plain documents and MUST NOT depend on any other layer"

forbid internal/domains 'internal/(http|middleware|apierrors|wire)|services' \
    "domains MUST NOT depend on the HTTP edge (handlers map domain errors, not the reverse)"

forbid internal/domains 'internal/storage/mongo/repository' \
    "domains MUST depend on internal/storage/interfaces, never on concrete Mongo repositories"

forbid internal/storage 'internal/(domains|http|middleware|wire)|services' \
    "storage is below the domain layer and MUST NOT import it"

forbid internal 'services' \
    "internal/* MUST NOT import services/* (services compose internal, not the reverse)"

echo ""
if [ "$VIOLATIONS" -eq 0 ]; then
    echo -e "${GREEN}✅ All architecture constraints satisfied${NC}"
    exit 0
fi
echo -e "${RED}❌ Found ${VIOLATIONS} architecture violation group(s)${NC}"
exit 1

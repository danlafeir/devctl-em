#!/usr/bin/env bash
# deploy.sh — interactive release script for em
# Creates a semver tag, collects release notes, builds all binaries,
# updates RELEASES.md, and pushes everything to main.
set -euo pipefail

# ── helpers ──────────────────────────────────────────────────────────────────

die() { echo "error: $*" >&2; exit 1; }
confirm() {
    local prompt="${1:-Continue?} [y/N] "
    local ans
    read -r -p "$prompt" ans
    [[ "${ans,,}" == "y" ]]
}

# ── require clean working tree ────────────────────────────────────────────────

if [[ -n "$(git status --porcelain)" ]]; then
    echo "Working tree is dirty:"
    git status --short
    echo ""
    confirm "Continue anyway?" || exit 0
fi

# ── compute current version ───────────────────────────────────────────────────

CURRENT_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [[ -z "$CURRENT_TAG" ]]; then
    CURRENT_VERSION="0.0.0"
    echo "No existing tags found. Starting at v0.0.0."
else
    CURRENT_VERSION="${CURRENT_TAG#v}"
    echo "Current version: $CURRENT_TAG"
fi

IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"

# ── bump type ─────────────────────────────────────────────────────────────────

echo ""
echo "Version bump:"
echo "  1) major  (v$((MAJOR+1)).0.0)"
echo "  2) minor  (v${MAJOR}.$((MINOR+1)).0)"
echo "  3) patch  (v${MAJOR}.${MINOR}.$((PATCH+1)))  [default]"
echo ""
read -r -p "Choice [1/2/3, default 3]: " BUMP_CHOICE

case "${BUMP_CHOICE:-3}" in
    1|major) NEW_VERSION="$((MAJOR+1)).0.0" ;;
    2|minor) NEW_VERSION="${MAJOR}.$((MINOR+1)).0" ;;
    3|patch|"") NEW_VERSION="${MAJOR}.${MINOR}.$((PATCH+1))" ;;
    *) die "Invalid choice: $BUMP_CHOICE" ;;
esac

NEW_TAG="v${NEW_VERSION}"
echo ""
echo "New version: $NEW_TAG"

# ── release notes ─────────────────────────────────────────────────────────────

echo ""
read -r -p "Release title (default: \"$NEW_TAG\"): " RELEASE_TITLE
RELEASE_TITLE="${RELEASE_TITLE:-$NEW_TAG}"

echo ""
echo "What changed? (blank line + Enter to finish)"
echo ""
CHANGE_LINES=()
while IFS= read -r line; do
    [[ -z "$line" && ${#CHANGE_LINES[@]} -gt 0 ]] && break
    CHANGE_LINES+=("$line")
done

if [[ ${#CHANGE_LINES[@]} -eq 0 ]]; then
    CHANGE_BODY="No details provided."
else
    CHANGE_BODY="$(printf '%s\n' "${CHANGE_LINES[@]}")"
fi

# ── confirm ───────────────────────────────────────────────────────────────────

echo ""
echo "────────────────────────────────────────"
echo "  Tag:   $NEW_TAG"
echo "  Title: $RELEASE_TITLE"
echo ""
echo "  Changes:"
while IFS= read -r line; do
    echo "    $line"
done <<< "$CHANGE_BODY"
echo "────────────────────────────────────────"
echo ""
confirm "Run tests, build, commit, and push?" || exit 0

# ── tests ─────────────────────────────────────────────────────────────────────

echo ""
echo "Running tests..."
make test

# ── update RELEASES.md ────────────────────────────────────────────────────────

RELEASE_DATE="$(date -u +%Y-%m-%d)"
RELEASES_FILE="RELEASES.md"

if [[ ! -f "$RELEASES_FILE" ]]; then
    cat > "$RELEASES_FILE" <<'HDR'
# Releases

Install or upgrade at any time:

```bash
curl -fsSL https://raw.githubusercontent.com/danlafeir/em/main/scripts/install.sh | bash
```

---

HDR
fi

# Prepend new entry after the header block (after the first ---)
ENTRY="## $NEW_TAG — $RELEASE_DATE

**$RELEASE_TITLE**

$CHANGE_BODY

---

"

# Build updated file: header block + new entry + existing entries
HEADER_END=$(grep -n "^---$" "$RELEASES_FILE" | head -1 | cut -d: -f1)
if [[ -n "$HEADER_END" ]]; then
    HEAD_BLOCK=$(head -n "$HEADER_END" "$RELEASES_FILE")
    TAIL_BLOCK=$(tail -n +"$((HEADER_END+1))" "$RELEASES_FILE")
    printf '%s\n\n%s%s\n' "$HEAD_BLOCK" "$ENTRY" "$TAIL_BLOCK" > "$RELEASES_FILE"
else
    printf '%s\n\n%s' "$(cat "$RELEASES_FILE")" "$ENTRY" > "$RELEASES_FILE"
fi

echo "Updated $RELEASES_FILE"

# ── tag ───────────────────────────────────────────────────────────────────────

git tag -a "$NEW_TAG" -m "$RELEASE_TITLE"
echo "Created tag $NEW_TAG"

# ── build ─────────────────────────────────────────────────────────────────────

echo ""
echo "Building all targets..."
make build-all

# ── commit and push ───────────────────────────────────────────────────────────

echo ""
echo "Committing..."
git add bin/ "$RELEASES_FILE"
git commit -m "Release $NEW_TAG: $RELEASE_TITLE"

echo "Pushing main + tag..."
git push origin main
git push origin "$NEW_TAG"

echo ""
echo "Done. $NEW_TAG is live."
echo "Users can upgrade with: em update"

#!/bin/bash
set -euo pipefail

MODULE="github.com/D-Andreev/cloudnative-patterns"
REPO="D-Andreev/cloudnative-patterns"

# Ensure we're on the main branch with latest changes
git checkout main
git pull origin main

LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo "Latest tag: $LATEST_TAG"
echo "Module:     $MODULE"

read -p "Enter new version (e.g. 0.1.1, without v prefix): " VERSION

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
	echo "Invalid version: $VERSION (expected semver like 0.1.1)"
	exit 1
fi

TAG="v${VERSION}"

if git rev-parse "$TAG" >/dev/null 2>&1; then
	echo "Tag $TAG already exists"
	exit 1
fi

echo "Running tests..."
make test

echo "Creating tag $TAG..."
git tag -a "$TAG" -m "Release $TAG"

echo "Pushing tag..."
git push origin "$TAG"

read -p "Release notes (optional, Enter to auto-generate): " NOTES
if [[ -z "$NOTES" ]]; then
	gh release create "$TAG" \
		--repo "$REPO" \
		--title "$TAG" \
		--generate-notes
else
	gh release create "$TAG" \
		--repo "$REPO" \
		--title "$TAG" \
		--notes "$NOTES"
fi

echo "Release $TAG published."
echo "Install with: go get ${MODULE}@${TAG}"

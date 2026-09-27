#!/bin/sh
# Draws the README's screenshots. nem sets each scene up on its simulated
# screen and writes it out as a page, at twice the size the README shows it
# (editor/screenshot_test.go), and a headless Firefox, in a profile of its
# own so it leaves any running browser alone, takes the picture.
set -eu
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
NEM_SCREENSHOTS="$tmp" go test ./editor -run '^TestScreenshots$' -count=1 >/dev/null
mkdir "$tmp/profile"
for page in "$tmp"/*.html; do
	name=$(basename "$page" .html)
	firefox --headless --no-remote --profile "$tmp/profile" \
		--window-size="$(cat "$tmp/$name.size")" \
		--screenshot "$PWD/docs/images/$name.png" "file://$page" >/dev/null 2>&1
	echo "docs/images/$name.png"
done

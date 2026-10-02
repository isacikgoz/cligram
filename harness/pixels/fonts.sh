#!/bin/sh
# fonts.sh fetches the fonts the pixel checks render with, whole: the
# box drawing glyphs are what is being tested, and subsetted web fonts
# leave them out. Each download is pinned by its checksum.
set -eu
cd "$(dirname "$0")"
mkdir -p fonts
fetch() { # url sha256 member output
	[ -f "fonts/$4" ] && return
	tmp=$(mktemp)
	curl -fsSL -o "$tmp" "$1"
	echo "$2  $tmp" | shasum -a 256 -c - >/dev/null
	unzip -p "$tmp" "$3" >"fonts/$4"
	rm -f "$tmp"
}
fetch https://github.com/JetBrains/JetBrainsMono/releases/download/v2.304/JetBrainsMono-2.304.zip \
	6f6376c6ed2960ea8a963cd7387ec9d76e3f629125bc33d1fdcd7eb7012f7bbf \
	fonts/ttf/JetBrainsMono-Regular.ttf JetBrainsMono-Regular.ttf
fetch https://github.com/dejavu-fonts/dejavu-fonts/releases/download/version_2_37/dejavu-fonts-ttf-2.37.zip \
	7576310b219e04159d35ff61dd4a4ec4cdba4f35c00e002a136f00e96a908b0a \
	dejavu-fonts-ttf-2.37/ttf/DejaVuSansMono.ttf DejaVuSansMono.ttf

#!/bin/sh
# Install a released airfs binary for the platform this runs on.
#
#   curl -fsSL https://raw.githubusercontent.com/sylvanld/airfs/main/scripts/get-airfs.sh | sh
#
# Configured by environment, not flags, because the usual invocation is a pipe
# into sh where flags need an awkward `-s --`:
#
#   AIRFS_VERSION       release tag to install    (default: the latest release)
#   AIRFS_INSTALL_DIR   where to write the binary (default: $HOME/.local/bin)
#
# POSIX sh only: whoever pipes this in chooses the shell, and on Debian and
# Ubuntu that is dash.
set -eu

REPO="sylvanld/airfs"
INSTALL_DIR="${AIRFS_INSTALL_DIR:-$HOME/.local/bin}"

die() {
	echo "get-airfs: $*" >&2
	exit 1
}

# Download $1 to $2, quietly, failing on any HTTP error rather than saving the
# error page. Neither tool is universally present, so support both.
download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	else
		wget -qO "$2" "$1"
	fi
}

if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
	die "needs curl or wget; install either one"
fi

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) die "unsupported OS '$(uname -s)'; build from source with 'go install github.com/$REPO/cmd/airfs@latest'" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "unsupported architecture '$(uname -m)'; build from source with 'go install github.com/$REPO/cmd/airfs@latest'" ;;
esac

# The tag names the assets, so an explicit version is used exactly as given
# rather than normalised into or out of a leading 'v'.
#
# Without one, the tag comes from where /releases/latest redirects to, rather
# than from the JSON API: the unauthenticated API allows 60 requests an hour per
# address, which a shared CI address can exhaust on its own.
latest_tag() {
	url="https://github.com/$REPO/releases/latest"
	if command -v curl >/dev/null 2>&1; then
		location=$(curl -fsSI -o /dev/null -w '%{redirect_url}' "$url")
	else
		location=$(wget -qS --max-redirect=0 -O /dev/null "$url" 2>&1 |
			sed -n 's/^[[:space:]]*[Ll]ocation:[[:space:]]*//p')
	fi
	# .../releases/tag/<tag> when a release exists; anything else means none does.
	printf '%s\n' "$location" | sed -n 's#.*/releases/tag/##p' | head -n 1
}

version="${AIRFS_VERSION:-}"
if [ -z "$version" ]; then
	version=$(latest_tag) ||
		die "cannot reach github.com; set AIRFS_VERSION to install a specific release"
	[ -n "$version" ] ||
		die "$REPO has no published release; set AIRFS_VERSION to install a specific tag"
fi

archive="airfs_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "get-airfs: downloading airfs $version for ${os}_${arch}"
download "$base/$archive" "$tmp/$archive" ||
	die "cannot download $base/$archive; check that '$version' is a published release"
download "$base/checksums.txt" "$tmp/checksums.txt" ||
	die "cannot download $base/checksums.txt"

# Verify before unpacking: an archive that does not match its published checksum
# is never written to disk outside the temporary directory.
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	die "needs sha256sum or shasum to verify the download; install either one"
fi

expected=$(sed -n "s/^\([0-9a-f]\{64\}\)[[:space:]]*[*]\{0,1\}$archive\$/\1/p" "$tmp/checksums.txt")
[ -n "$expected" ] || die "$archive is not listed in checksums.txt"

actual=$(sha256 "$tmp/$archive")
[ "$actual" = "$expected" ] ||
	die "checksum mismatch for $archive: expected $expected, got $actual"

tar -xzf "$tmp/$archive" -C "$tmp" airfs || die "cannot unpack $archive"

mkdir -p "$INSTALL_DIR" || die "cannot create $INSTALL_DIR"
install -m 755 "$tmp/airfs" "$INSTALL_DIR/airfs" ||
	die "cannot write $INSTALL_DIR/airfs"

echo "get-airfs: installed airfs $version to $INSTALL_DIR/airfs"

case ":$PATH:" in
*":$INSTALL_DIR:"*) echo "get-airfs: run 'airfs doctor' to check this host can mount" ;;
*) echo "get-airfs: $INSTALL_DIR is not on your PATH; add it, then run 'airfs doctor'" ;;
esac

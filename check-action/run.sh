#!/usr/bin/env bash
# The check-action step (AC181): resolve the release, download the runner's
# archive and checksums.txt, verify the archive's sha256, then run
# `lyceum check --ci` in the workspace and exit with its status.
#
# Inputs (environment):
#   LYCEUM_VERSION            the action's `version` input; empty means "from the action's ref"
#   LYCEUM_ACTION_REF         github.action_ref; when empty, read from GITHUB_ACTION_PATH
#   LYCEUM_ACTION_REPOSITORY  github.action_repository; when empty, read from GITHUB_ACTION_PATH
#   RUNNER_OS, RUNNER_ARCH, RUNNER_TEMP, GITHUB_WORKSPACE, GITHUB_ACTION_PATH  set by the runner
# Overrides, for tests:
#   LYCEUM_ACTION_BASE_URL    release download base (default https://github.com/<repo>/releases/download)
#   LYCEUM_ACTION_GIT_URL     where tags are listed (default https://github.com/<repo>.git)
set -euo pipefail

fail() {
	echo "lyceum check-action: $*" >&2
	exit 1
}

case "${RUNNER_OS:-}" in
Linux) os=linux ;;
macOS) os=darwin ;;
*) fail "unsupported runner OS '${RUNNER_OS:-}': Lyceum releases are built for Linux and macOS (X64, ARM64)" ;;
esac
case "${RUNNER_ARCH:-}" in
X64) arch=amd64 ;;
ARM64) arch=arm64 ;;
*) fail "unsupported runner architecture '${RUNNER_ARCH:-}': Lyceum releases are built for X64 and ARM64 (Linux, macOS)" ;;
esac

# Composite actions can see an empty github.action_ref and github.action_repository;
# the runner checks the action out at _actions/<owner>/<repo>/<ref>/check-action.
ref="${LYCEUM_ACTION_REF:-}"
repo="${LYCEUM_ACTION_REPOSITORY:-}"
path="${GITHUB_ACTION_PATH:-}"
if [[ "$path" == */_actions/*/*/*/check-action ]]; then
	rest="${path#*/_actions/}"
	rest="${rest%/check-action}"
	owner="${rest%%/*}"
	rest="${rest#*/}"
	[ -n "$repo" ] || repo="$owner/${rest%%/*}"
	[ -n "$ref" ] || ref="${rest#*/}"
fi
[ -n "$repo" ] || repo=oritromax/lyceum
base_url="${LYCEUM_ACTION_BASE_URL:-https://github.com/$repo/releases/download}"
git_url="${LYCEUM_ACTION_GIT_URL:-https://github.com/$repo.git}"

# tags prints "<tag> <commit>" for every tag, annotated tags peeled to their commit.
tags() {
	git ls-remote --tags "$git_url" | awk '
		{ t = $2; sub("^refs/tags/", "", t) }
		t ~ /\^\{\}$/ { sub(/\^\{\}$/, "", t); peeled[t] = $1; next }
		{ plain[t] = $1 }
		END { for (t in plain) print t, (t in peeled ? peeled[t] : plain[t]) }'
}

# major_release vN: the vN.x.y release the moving vN tag points at (the release
# job moves vN only after that release is built and verified).
major_release() {
	local list commit
	list="$(tags)" || fail "cannot list the tags of $git_url"
	commit="$(awk -v t="$1" '$1 == t { print $2 }' <<<"$list")"
	[ -n "$commit" ] || fail "no tag $1 in $git_url"
	awk -v c="$commit" -v m="$1" '$2 == c && index($1, m ".") == 1 && $1 ~ /^v[0-9]+\.[0-9]+\.[0-9]+$/ { print $1 }' <<<"$list" |
		sort -V | tail -n 1 | grep . || fail "tag $1 points at no $1.x.y release"
}

# latest_major: the highest vN tag.
latest_major() {
	local list
	list="$(tags)" || fail "cannot list the tags of $git_url"
	awk '$1 ~ /^v[0-9]+$/ { print $1 }' <<<"$list" | sort -V | tail -n 1 | grep . ||
		fail "no released major version (vN tag) in $git_url"
}

# no_major: true while the repo has no vN tag at all.
no_major() {
	local list
	list="$(tags)" || fail "cannot list the tags of $git_url"
	! awk '$1 ~ /^v[0-9]+$/ { found = 1 } END { exit !found }' <<<"$list"
}

# newest_release: the highest vX.Y.Z[-pre] tag by semver precedence, prereleases
# included (1.0.0-rc.2 > 1.0.0-rc.1, 1.0.0 > 1.0.0-rc.9). Used only while no vN tag exists.
newest_release() {
	local list
	list="$(tags)" || fail "cannot list the tags of $git_url"
	awk '
		function ident(a, b) {
			if (a ~ /^[0-9]+$/ && b ~ /^[0-9]+$/) return (a + 0 > b + 0) - (a + 0 < b + 0)
			if (a ~ /^[0-9]+$/) return -1
			if (b ~ /^[0-9]+$/) return 1
			return (a > b) - (a < b)
		}
		function cmp(a, b,   pa, pb, ca, cb, xa, xb, na, nb, i, c) {
			pa = index(a, "-"); pb = index(b, "-")
			ca = pa ? substr(a, 1, pa - 1) : a; cb = pb ? substr(b, 1, pb - 1) : b
			split(ca, xa, "."); split(cb, xb, ".")
			for (i = 1; i <= 3; i++) if ((c = ident(xa[i], xb[i])) != 0) return c
			if (!pa || !pb) return (!pa) - (!pb)
			na = split(substr(a, pa + 1), xa, "."); nb = split(substr(b, pb + 1), xb, ".")
			for (i = 1; i <= na && i <= nb; i++) if ((c = ident(xa[i], xb[i])) != 0) return c
			return (na > nb) - (na < nb)
		}
		$1 ~ /^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$/ {
			v = substr($1, 2)
			if (best == "" || cmp(v, best) > 0) best = v
		}
		END { if (best != "") print "v" best }' <<<"$list" | grep . || fail "no release tag (vX.Y.Z) in $git_url"
}

want="${LYCEUM_VERSION:-}"
if [ -z "$want" ]; then
	if [[ "$ref" =~ ^v[0-9]+(\.[0-9]+\.[0-9]+([-+].*)?)?$ ]]; then
		want="$ref"
	else
		want=latest
	fi
fi
case "$want" in
latest)
	if no_major; then
		tag="$(newest_release)"
	else
		major="$(latest_major)"
		tag="$(major_release "$major")"
	fi
	;;
v[0-9]*.*.* | [0-9]*.*.*) tag="v${want#v}" ;;
v[0-9]*)
	if no_major; then
		tag="$(newest_release)"
	else
		tag="$(major_release "$want")"
	fi
	;;
*) fail "version '$want' is not latest, vN, or a release tag like v1.2.3" ;;
esac
version="${tag#v}"
archive="lyceum_${version}_${os}_${arch}.tar.gz"
echo "lyceum check-action: $tag ($archive)"

dir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/lyceum.XXXXXX")"
for f in "$archive" checksums.txt; do
	curl -fsSL --retry 3 -o "$dir/$f" "$base_url/$tag/$f" || fail "cannot download $base_url/$tag/$f"
done

want_sum="$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$dir/checksums.txt")"
[ -n "$want_sum" ] || fail "checksums.txt of $tag has no entry for $archive"
if command -v sha256sum >/dev/null; then
	got_sum="$(sha256sum "$dir/$archive" | awk '{ print $1 }')"
else
	got_sum="$(shasum -a 256 "$dir/$archive" | awk '{ print $1 }')"
fi
[ "$got_sum" = "$want_sum" ] || fail "checksum mismatch for $archive: expected $want_sum, got $got_sum"

tar -xzf "$dir/$archive" -C "$dir" lyceum || fail "$archive has no lyceum binary"
cd "${GITHUB_WORKSPACE:-.}"
exec "$dir/lyceum" check --ci

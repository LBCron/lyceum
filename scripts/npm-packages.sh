#!/bin/sh
# Turns goreleaser's archives into the npm packages (esbuild pattern):
#   <out>/cli-<os>-<arch>/  @lyceum-spec/cli-<os>-<arch>, the binary under bin/
#   <out>/lyceum/           lyceum, the shim, with the four as optionalDependencies
# Usage: scripts/npm-packages.sh <version> <dist dir> <out dir>
set -eu

if [ $# -ne 3 ]; then
	echo "usage: $0 <version> <dist dir> <out dir>" >&2
	exit 2
fi
version=${1#v}
dist=$2
out=$3
root=$(cd "$(dirname "$0")/.." && pwd)

mkdir -p "$out"
out=$(cd "$out" && pwd)

# goreleaser os_arch -> npm os-cpu
for pair in linux_amd64:linux-x64 linux_arm64:linux-arm64 darwin_amd64:darwin-x64 darwin_arm64:darwin-arm64; do
	goplat=${pair%%:*}
	plat=${pair#*:}
	archive="$dist/lyceum_${version}_${goplat}.tar.gz"
	if [ ! -f "$archive" ]; then
		echo "$0: $archive not found" >&2
		exit 1
	fi
	pkg="$out/cli-$plat"
	rm -rf "$pkg"
	mkdir -p "$pkg/bin"
	tar -xzf "$archive" -C "$pkg/bin" lyceum
	chmod 755 "$pkg/bin/lyceum"
	cp "$root/LICENSE" "$pkg/"
	node -e '
		const [version, plat, file] = process.argv.slice(1);
		const [os, cpu] = plat.split("-");
		const pkg = {
			name: `@lyceum-spec/cli-${plat}`,
			version,
			description: `The lyceum binary for ${plat}. Install the lyceum package, not this one.`,
			license: "MIT",
			repository: { type: "git", url: "git+https://github.com/oritromax/lyceum.git" },
			os: [os],
			cpu: [cpu],
			files: ["bin/"],
		};
		require("node:fs").writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
	' "$version" "$plat" "$pkg/package.json"
done

main="$out/lyceum"
rm -rf "$main"
mkdir -p "$main/bin"
cp "$root/bin/lyceum.js" "$main/bin/"
chmod 755 "$main/bin/lyceum.js"
cp "$root/README.md" "$root/LICENSE" "$main/"
node -e '
	const [version, src, file] = process.argv.slice(1);
	const pkg = JSON.parse(require("node:fs").readFileSync(src, "utf8"));
	pkg.version = version;
	pkg.optionalDependencies = {};
	for (const plat of ["linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64"]) {
		pkg.optionalDependencies[`@lyceum-spec/cli-${plat}`] = version;
	}
	require("node:fs").writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
' "$version" "$root/package.json" "$main/package.json"

echo "$out/cli-linux-x64 $out/cli-linux-arm64 $out/cli-darwin-x64 $out/cli-darwin-arm64 $out/lyceum"

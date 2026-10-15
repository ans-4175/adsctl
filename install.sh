#!/bin/sh
# Install the published macOS/Linux CLI into the user's adsctl home.
set -eu
umask 077

fail() { printf 'adsctl install: %s\n' "$*" >&2; exit 1; }
quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

adsctl_version=${ADSCTL_VERSION:-latest}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail '--version needs a release version'
      adsctl_version=$2
      shift 2
      ;;
    -h|--help)
      printf '%s\n' 'Usage: sh install.sh [--version vX.Y.Z]' \
        'ADSCTL_HOME changes the installation directory (default: ~/.adsctl).' \
        'ADSCTL_VERSION selects a release (default: latest stable release).'
      exit 0
      ;;
    *) fail "Unknown option: $1" ;;
  esac
done

command -v curl >/dev/null 2>&1 || fail 'curl is required'
command -v tar >/dev/null 2>&1 || fail 'tar is required'
if command -v sha256sum >/dev/null 2>&1; then
  adsctl_hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  adsctl_hash_tool=shasum
else
  fail 'sha256sum or shasum is required for checksum verification'
fi

case "$(uname -s)" in
  Darwin) adsctl_os=darwin ;;
  Linux) adsctl_os=linux ;;
  *) fail 'This installer supports macOS and Linux; use a release ZIP on Windows.' ;;
esac
case "$(uname -m)" in
  x86_64|amd64) adsctl_arch=amd64 ;;
  arm64|aarch64) adsctl_arch=arm64 ;;
  *) fail 'Supported architectures are x86-64 and ARM64.' ;;
esac

if [ -n "${ADSCTL_HOME:-}" ]; then
  adsctl_dir=$ADSCTL_HOME
else
  [ -n "${HOME:-}" ] || fail 'Set ADSCTL_HOME when HOME is unavailable'
  adsctl_dir=$HOME/.adsctl
fi
case "$adsctl_dir" in
  /*) ;;
  *) fail 'ADSCTL_HOME must be an absolute path' ;;
esac

adsctl_releases=https://github.com/imrenagicom/adsctl/releases
download() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 15 --max-time 180 "$@"
}
if [ "$adsctl_version" = latest ]; then
  adsctl_latest=$(download --output /dev/null --write-out '%{url_effective}' "$adsctl_releases/latest")
  case "$adsctl_latest" in
    "$adsctl_releases/tag/"*) adsctl_version=${adsctl_latest##*/} ;;
    *) fail 'Could not resolve the latest GitHub release' ;;
  esac
fi
adsctl_version=${adsctl_version#v}
printf '%s\n' "$adsctl_version" | LC_ALL=C grep -Eq \
  '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$' \
  || fail 'Expected a semantic release version such as v0.4.0'

adsctl_archive=adsctl_${adsctl_version}_${adsctl_os}_${adsctl_arch}.tar.gz
adsctl_url=$adsctl_releases/download/v$adsctl_version
adsctl_work=$(mktemp -d "${TMPDIR:-/tmp}/adsctl-install.XXXXXX")
adsctl_candidate=
cleanup() {
  rm -rf "$adsctl_work"
  if [ -n "$adsctl_candidate" ]; then rm -f "$adsctl_candidate"; fi
}
trap cleanup 0
trap 'exit 1' 1 2 3 15

printf 'Downloading adsctl v%s for %s/%s...\n' "$adsctl_version" "$adsctl_os" "$adsctl_arch"
download --output "$adsctl_work/$adsctl_archive" "$adsctl_url/$adsctl_archive"
download --output "$adsctl_work/checksums.txt" "$adsctl_url/checksums.txt"
adsctl_expected=$(awk -v name="$adsctl_archive" '$2 == name || $2 == "*" name {print $1}' "$adsctl_work/checksums.txt")
[ "${#adsctl_expected}" -eq 64 ] || fail 'Release checksum is missing or ambiguous'
if [ "$adsctl_hash_tool" = sha256sum ]; then
  adsctl_actual=$(sha256sum "$adsctl_work/$adsctl_archive" | awk '{print $1}')
else
  adsctl_actual=$(shasum -a 256 "$adsctl_work/$adsctl_archive" | awk '{print $1}')
fi
[ "$adsctl_actual" = "$adsctl_expected" ] || fail 'Checksum mismatch; existing installation was preserved'
tar -xzf "$adsctl_work/$adsctl_archive" -C "$adsctl_work" adsctl
[ -f "$adsctl_work/adsctl" ] && [ ! -L "$adsctl_work/adsctl" ] || fail 'Archive does not contain a regular adsctl binary'
chmod 755 "$adsctl_work/adsctl"
"$adsctl_work/adsctl" version >/dev/null || fail 'Downloaded executable cannot run on this host'
adsctl_probe=$(ADSCTL_HOME=/__adsctl_installer_probe__ "$adsctl_work/adsctl" config path) \
  || fail 'Downloaded executable cannot resolve its configuration'
printf '%s\n' "$adsctl_probe" | grep -F '"/__adsctl_installer_probe__/config.yaml"' >/dev/null \
  || fail 'This release predates ADSCTL_HOME support; select a newer release. Existing installation was preserved.'

mkdir -p "$adsctl_dir/bin" "$adsctl_dir/secrets" "$adsctl_dir/state"
[ ! -d "$adsctl_dir/bin/adsctl" ] || fail 'The executable destination is a directory; installation was preserved'
[ ! -d "$adsctl_dir/env" ] || fail 'The shell setup destination is a directory; installation was preserved'
adsctl_candidate=$(mktemp "$adsctl_dir/bin/.adsctl-install.XXXXXX")
cp "$adsctl_work/adsctl" "$adsctl_candidate"
chmod 755 "$adsctl_candidate"
mv -f "$adsctl_candidate" "$adsctl_dir/bin/adsctl"
adsctl_candidate=

# Noclobber also preserves configuration created concurrently with installation.
if [ ! -e "$adsctl_dir/config.yaml" ] && [ ! -L "$adsctl_dir/config.yaml" ]; then
  (set -C; printf 'version: 1\nprofiles: {}\nportfolios: {}\n' > "$adsctl_dir/config.yaml") \
    || fail 'Could not create starter configuration; no existing configuration was overwritten'
fi

adsctl_candidate=$(mktemp "$adsctl_dir/.env-install.XXXXXX")
{
  printf '# Generated by the adsctl installer. Source from bash, zsh, or sh.\n'
  printf 'export ADSCTL_HOME=%s\n' "$(quote "$adsctl_dir")"
  printf '%s\n' 'case ":$PATH:" in' \
    '  *":$ADSCTL_HOME/bin:"*) ;;' \
    '  *) export PATH="$ADSCTL_HOME/bin:$PATH" ;;' \
    'esac'
} > "$adsctl_candidate"
mv -f "$adsctl_candidate" "$adsctl_dir/env"
adsctl_candidate=

printf '\nInstalled adsctl v%s in %s/bin/adsctl\n' "$adsctl_version" "$adsctl_dir"
printf 'Configuration: %s/config.yaml\nSecrets: %s/secrets/\nState: %s/state/\n' \
  "$adsctl_dir" "$adsctl_dir" "$adsctl_dir"
printf '\nAdd adsctl to PATH in a bash, zsh, or sh session:\n  . %s\n' "$(quote "$adsctl_dir/env")"
adsctl_source_line=". $(quote "$adsctl_dir/env")"
case "${SHELL:-}" in
  */zsh) adsctl_rc='"$HOME/.zshrc"' ;;
  */bash)
    if [ "$adsctl_os" = darwin ]; then adsctl_rc='"$HOME/.bash_profile"'; else adsctl_rc='"$HOME/.bashrc"'; fi
    ;;
  *) adsctl_rc= ;;
esac
if [ -n "$adsctl_rc" ]; then
  printf '\nFor future sessions, add the source line once and reload:\n'
  printf '  printf '\''\\n%%s\\n'\'' %s >> %s\n' "$(quote "$adsctl_source_line")" "$adsctl_rc"
  printf '  . %s\n' "$adsctl_rc"
else
  printf '\nFor future bash/zsh sessions, add the source line to your shell startup file.\n'
fi
printf '\nThen run:\n  adsctl version\n  adsctl config path\n'
printf '\nExisting configuration, secrets, and state are preserved when reinstalling.\n'
printf 'For installations using older default directories, move that data or select it with --config/--state-dir.\n'

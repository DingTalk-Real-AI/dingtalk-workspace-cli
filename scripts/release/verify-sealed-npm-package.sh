#!/bin/sh
set -eu

# Execute the sealed version's installation contract without publishing rights.
# Old verifiers invoke npm pack themselves. Supply the already sealed bytes at
# that boundary, rather than repacking or borrowing today's Skill target list.
SOURCE_ROOT="${1:?sealed source root required}"
VERSION="${2:?release version required}"
TARBALL="${DWS_VERIFY_NPM_TARBALL:?sealed npm tarball required}"
DIST="${DWS_PACKAGE_DIST_DIR:?package distribution directory required}"
test -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" || {
  echo 'Installation verification must not hold OIDC publication authority' >&2
  exit 1
}
test -f "$TARBALL"
test -f "$SOURCE_ROOT/scripts/release/verify-package-managers.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/dws-sealed-verifier.XXXXXX")"
trap 'rm -rf "$work"' EXIT HUP INT TERM
DWS_VERIFY_REAL_NPM="$(command -v npm)"
DWS_VERIFY_FIXED_TARBALL="$TARBALL"
DWS_VERIFY_STAGE="$(CDPATH= cd -- "$DIST/npm/dingtalk-workspace-cli" && pwd -P)"
export DWS_VERIFY_REAL_NPM DWS_VERIFY_FIXED_TARBALL DWS_VERIFY_STAGE
cat > "$work/npm" <<'SH'
#!/bin/sh
set -eu
case "${1:-}" in
  pack)
    shift
    for arg do
      case "$arg" in --silent|--ignore-scripts) ;; *) echo "Unexpected sealed npm pack argument: $arg" >&2; exit 1 ;; esac
    done
    test "$(pwd -P)" = "$DWS_VERIFY_STAGE"
    cp "$DWS_VERIFY_FIXED_TARBALL" "$DWS_VERIFY_STAGE/dws-verified-input.tgz"
    printf '%s\n' dws-verified-input.tgz
    ;;
  install)
    test "$#" -eq 3
    test "$2" = -g
    cmp -s "$3" "$DWS_VERIFY_FIXED_TARBALL" || {
      echo 'Verifier attempted to install different npm bytes' >&2; exit 1;
    }
    exec "$DWS_VERIFY_REAL_NPM" "$@"
    ;;
  *) exec "$DWS_VERIFY_REAL_NPM" "$@" ;;
esac
SH
chmod +x "$work/npm"
PATH="$work:$PATH" sh "$SOURCE_ROOT/scripts/release/verify-package-managers.sh" \
  --npm-only --expected-version "$VERSION"

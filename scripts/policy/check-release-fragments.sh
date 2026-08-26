#!/bin/sh
set -eu

# Fragment names are matched with ASCII ranges, so keep collation deterministic.
LC_ALL=C
export LC_ALL

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

usage() { printf '%s\n' 'usage: check-release-fragments.sh BASE HEAD' >&2; }

[ "$#" -eq 2 ] || { usage; exit 2; }
base="$(git -C "$ROOT" rev-parse --verify --quiet "$1^{commit}")" || { usage; exit 2; }
head="$(git -C "$ROOT" rev-parse --verify --quiet "$2^{commit}")" || { usage; exit 2; }
merge_base="$(git -C "$ROOT" merge-base "$base" "$head")"
tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-fragment-policy.XXXXXX")"
cleanup() { rm -rf "$tmp_root"; }
trap cleanup EXIT HUP INT TERM

git -C "$ROOT" diff --no-ext-diff --find-renames --name-status "$merge_base" "$head" >"$tmp_root/status"

normalize_notes() {
  awk '
    /^[[:space:]]*$/ && !started { next }
    { started = 1; lines[++count] = $0 }
    END {
      while (count > 0 && lines[count] ~ /^[[:space:]]*$/) count--
      for (line_no = 1; line_no <= count; line_no++) print lines[line_no]
    }
  ' "$1"
}

extract_release_notes() {
  git -C "$ROOT" show "$1:CHANGELOG.md" |
    awk -v heading="## [$2] - " '
      index($0, heading) == 1 { found = 1; next }
      found && /^## / { exit }
      found { print }
    '
}

release_heading() {
  git -C "$ROOT" show "$1:CHANGELOG.md" |
    awk -v heading="## [$2] - " 'index($0, heading) == 1 { print }'
}

changelog_without_release_notes() {
  git -C "$ROOT" show "$1:CHANGELOG.md" |
    awk -v heading="## [$2] - " '
      index($0, heading) == 1 { print; skipped = 1; next }
      skipped && /^## / { skipped = 0 }
      !skipped { print }
    '
}

# A pre-tag beta amendment must preserve the original seal verbatim except for
# appending newly archived fragment bodies to their existing categories. These
# helpers canonicalize the renderer's category format before comparing it.
canonicalize_fragment_notes() {
  awk '
    BEGIN { order[1] = "Added"; order[2] = "Changed"; order[3] = "Deprecated"; order[4] = "Removed"; order[5] = "Fixed"; order[6] = "Security" }
    function trim(value) {
      sub(/^[\n]+/, "", value)
      sub(/[\n]+$/, "", value)
      return value
    }
    function flush() {
      if (category == "") return
      value = trim(body)
      if (value == "" || seen[category]++) invalid = 1
      blocks[category] = value
      category = ""
      body = ""
    }
    /^### / {
      flush()
      candidate = substr($0, 5)
      if (candidate !~ /^(Added|Changed|Deprecated|Removed|Fixed|Security)$/) {
        invalid = 1
        next
      }
      category = candidate
      next
    }
    {
      if (category == "") {
        if ($0 !~ /^[[:space:]]*$/) invalid = 1
        next
      }
      body = body $0 "\n"
    }
    END {
      flush()
      if (invalid) exit 1
      for (i = 1; i <= 6; i++) {
        category = order[i]
        if (category in blocks) printf "### %s\n\n%s\n\n", category, blocks[category]
      }
    }
  ' "$1"
}

merge_canonical_fragment_notes() {
  awk '
    BEGIN { order[1] = "Added"; order[2] = "Changed"; order[3] = "Deprecated"; order[4] = "Removed"; order[5] = "Fixed"; order[6] = "Security" }
    function trim(value) {
      sub(/^[\n]+/, "", value)
      sub(/[\n]+$/, "", value)
      return value
    }
    function flush() {
      if (category == "") return
      value = trim(body)
      if (value == "") invalid = 1
      if (part == 1) before[category] = value
      else if (part == 2) addition[category] = value
      else invalid = 1
      category = ""
      body = ""
    }
    FNR == 1 {
      if (started) flush()
      started = 1
      part++
    }
    /^### / {
      flush()
      category = substr($0, 5)
      if (category !~ /^(Added|Changed|Deprecated|Removed|Fixed|Security)$/) invalid = 1
      next
    }
    { body = body $0 "\n" }
    END {
      flush()
      if (invalid || part != 2) exit 1
      for (i = 1; i <= 6; i++) {
        category = order[i]
        if (!(category in before) && !(category in addition)) continue
        printf "### %s\n\n", category
        if (category in before) printf "%s\n\n", before[category]
        if (category in addition) printf "%s\n\n", addition[category]
      }
    }
  ' "$1" "$2"
}

archive_changed=false
if awk -F '\t' '{ for (field = 2; field <= NF; field++) if ($field ~ /^\.changes\/released\//) found = 1 } END { exit !found }' "$tmp_root/status"; then
  archive_changed=true
fi

if [ "$archive_changed" = true ]; then
  new_release_version="$(git -C "$ROOT" diff --no-ext-diff --unified=0 "$merge_base" "$head" -- CHANGELOG.md | sed -n 's/^+## \[\([0-9][0-9.]*\(-beta\.[1-9][0-9]*\)\{0,1\}\)\] - .*/\1/p')"
  new_release_version_count="$(printf '%s\n' "$new_release_version" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
  archival_mode=seal
  if [ "$new_release_version_count" -eq 1 ]; then
    release_version="$new_release_version"
  else
    # An amendment keeps the existing heading. Derive the one allowed target
    # from the literal archive directory, then verify every move below.
    release_version="$(awk -F '\t' '$1 == "R100" && NF == 3 && $3 ~ /^\.changes\/released\// { path = $3; sub(/^\.changes\/released\//, "", path); sub(/\/.*/, "", path); print path }' "$tmp_root/status" | sort -u)"
    [ "$(printf '%s\n' "$release_version" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq 1 ] || {
      printf '%s\n' 'error: release-fragment archival requires exactly one newly added versioned CHANGELOG section or one untagged beta amendment target' >&2
      exit 1
    }
    printf '%s\n' "$release_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-beta\.[1-9][0-9]*$' || {
      printf '%s\n' 'error: only an existing beta CHANGELOG section may receive a pre-tag fragment amendment' >&2
      exit 1
    }
    base_heading="$(release_heading "$merge_base" "$release_version")"
    head_heading="$(release_heading "$head" "$release_version")"
    [ "$(printf '%s\n' "$base_heading" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq 1 ] && [ "$base_heading" = "$head_heading" ] || {
      printf '%s\n' 'error: beta fragment amendment requires one unchanged pre-existing CHANGELOG heading' >&2
      exit 1
    }
    changelog_without_release_notes "$merge_base" "$release_version" >"$tmp_root/base-changelog-without-amendment"
    changelog_without_release_notes "$head" "$release_version" >"$tmp_root/head-changelog-without-amendment"
    if ! cmp -s "$tmp_root/base-changelog-without-amendment" "$tmp_root/head-changelog-without-amendment"; then
      printf '%s\n' 'error: beta fragment amendment may change only its existing beta CHANGELOG section' >&2
      exit 1
    fi
    if git -C "$ROOT" rev-parse --verify --quiet "refs/tags/v$release_version^{commit}" >/dev/null; then
      printf 'error: beta fragment amendment is forbidden after tag v%s exists\n' "$release_version" >&2
      exit 1
    fi
    archival_mode=beta-amendment
  fi
  # The archive directory is matched as a literal prefix, never as a regex:
  # interpolating the version into one would make `.` match any character, so
  # `1.0.1-beta.1` would also admit `.changes/released/1x0x1-betaX1/` and break
  # the documented audit trail. Only the fragment basename, whose character class
  # is fixed, stays a pattern.
  if ! awk -F '\t' -v prefix=".changes/released/$release_version/" '
    $1 == "M" && $2 == "CHANGELOG.md" && NF == 2 { changelog = 1; next }
    $1 == "R100" && NF == 3 && $2 ~ /^\.changes\/[a-z0-9][a-z0-9._-]*\.md$/ && index($3, prefix) == 1 {
      target = substr($3, length(prefix) + 1)
      if (target !~ /^[a-z0-9][a-z0-9._-]*\.md$/) { invalid = 1; next }
      source = $2; sub(/^.*\//, "", source)
      if (source != target) invalid = 1
      moved++; next
    }
    { invalid = 1 }
    END { exit !(changelog && moved > 0 && !invalid) }
  ' "$tmp_root/status"; then
    printf '%s\n' 'error: release fragments must be unchanged R100 moves from .changes/<name>.md to .changes/released/<new-version>/<name>.md in the matching release-seal or untagged beta-amendment PR' >&2
    exit 1
  fi
  source_changes="$tmp_root/source-changes"
  mkdir -p "$source_changes"
  git -C "$ROOT" ls-tree -r --name-only "$merge_base" -- .changes |
    while IFS= read -r path; do
      case "$path" in
        .changes/*.md)
          name="${path#.changes/}"
          mkdir -p "$(dirname "$source_changes/$name")"
          git -C "$ROOT" show "$merge_base:$path" >"$source_changes/$name"
          ;;
      esac
    done
  "$ROOT/scripts/release/render-release-fragments.sh" "$source_changes" >"$tmp_root/expected-notes"
  if [ "$archival_mode" = beta-amendment ]; then
    extract_release_notes "$merge_base" "$release_version" >"$tmp_root/base-notes"
    extract_release_notes "$head" "$release_version" >"$tmp_root/head-notes"
    if ! canonicalize_fragment_notes "$tmp_root/base-notes" >"$tmp_root/base-notes.canonical" ||
      ! canonicalize_fragment_notes "$tmp_root/expected-notes" >"$tmp_root/amendment-notes.canonical" ||
      ! merge_canonical_fragment_notes "$tmp_root/base-notes.canonical" "$tmp_root/amendment-notes.canonical" >"$tmp_root/expected-notes.merged" ||
      ! canonicalize_fragment_notes "$tmp_root/head-notes" >"$tmp_root/actual-notes.canonical"; then
      printf '%s\n' 'error: beta fragment amendment requires canonical rendered fragment categories' >&2
      exit 1
    fi
    normalize_notes "$tmp_root/expected-notes.merged" >"$tmp_root/expected-notes.normalized"
    normalize_notes "$tmp_root/actual-notes.canonical" >"$tmp_root/actual-notes.normalized"
  else
    extract_release_notes "$head" "$release_version" >"$tmp_root/actual-section"
    case "$release_version" in
      *-beta.*)
        cp "$tmp_root/actual-section" "$tmp_root/actual-notes"
        ;;
      *)
      stable_beta="$(sed -n 's/^### Changes since `\(v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*-beta\.[1-9][0-9]*\)`$/\1/p' "$tmp_root/actual-section")"
      [ "$(printf '%s\n' "$stable_beta" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq 1 ] || {
        printf '%s\n' 'error: stable release-seal with archived fragments requires exactly one ### Changes since `vX.Y.Z-beta.N` boundary' >&2
        exit 1
      }
      . "$ROOT/scripts/release/release-lib.sh"
      [ "$(release_core_tag "$stable_beta")" = "v$release_version" ] || {
        printf 'error: stable post-beta fragment boundary %s does not match release v%s\n' "$stable_beta" "$release_version" >&2
        exit 1
      }
      awk '
        found { print }
        /^### Changes since `v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*-beta\.[1-9][0-9]*`$/ { found = 1 }
      ' "$tmp_root/actual-section" >"$tmp_root/actual-notes"
        ;;
    esac
    normalize_notes "$tmp_root/expected-notes" >"$tmp_root/expected-notes.normalized"
    normalize_notes "$tmp_root/actual-notes" >"$tmp_root/actual-notes.normalized"
  fi
  if ! cmp -s "$tmp_root/expected-notes.normalized" "$tmp_root/actual-notes.normalized"; then
    printf '%s\n' 'error: release CHANGELOG section does not exactly match the rendered active release fragments' >&2
    diff -u "$tmp_root/expected-notes.normalized" "$tmp_root/actual-notes.normalized" >&2 || true
    exit 1
  fi
else
  if awk -F '\t' '{ for (field = 2; field <= NF; field++) if ($field ~ /^\.changes\/released\//) invalid = 1 } END { exit !invalid }' "$tmp_root/status"; then
    printf '%s\n' 'error: archived release fragments are immutable outside their release-seal PR' >&2
    exit 1
  fi
	if awk -F '\t' '
		$1 == "D" && $2 ~ /^\.changes\/[a-z0-9][a-z0-9._-]*\.md$/ { invalid = 1 }
		$1 ~ /^R[0-9]+$/ && $2 ~ /^\.changes\/[a-z0-9][a-z0-9._-]*\.md$/ { invalid = 1 }
		END { exit !invalid }
	' "$tmp_root/status"; then
		printf '%s\n' 'error: active release fragments may be deleted or renamed only by the matching release-seal archival move' >&2
		exit 1
	fi
fi

# `.changes/released/**` carries its own immutability and release-seal checks
# above, so every other `.changes` change must revalidate the top-level tree.
# This trigger must not be narrowed to single-level paths or to the legal
# fragment name pattern: git records no diff entry for a directory itself, so
# adding `.changes/foo/bar.md` only ever shows the nested path, and an illegally
# named or non-regular entry only ever shows its own path. Either one would skip
# validation here and then break the next unrelated PR that adds a legal
# fragment.
git -C "$ROOT" diff --no-ext-diff --name-only "$merge_base" "$head" -- .changes >"$tmp_root/changes-paths"

changes_tree_changed=false
if awk '
  $0 ~ /^\.changes\/released\// { next }
  { found = 1 }
  END { exit !found }
' "$tmp_root/changes-paths"; then
  changes_tree_changed=true
fi

# Re-rendering is driven by fragment changes only. `.changes/README.md` is the
# contributor contract rather than release content, and it may be edited while no
# fragment is pending, which the renderer would reject as an empty fragment set.
fragment_changed=false
if awk '
  $0 ~ /^\.changes\/released\// { next }
  $0 == ".changes/README.md" { next }
  { found = 1 }
  END { exit !found }
' "$tmp_root/changes-paths"; then
  fragment_changed=true
fi

if [ "$changes_tree_changed" = true ]; then
  # `.changes` itself must stay a directory: replacing it with a blob or a
  # symlink leaves the child listing below empty, which would report no invalid
  # entries while silently discarding every fragment.
  changes_root_type="$(git -C "$ROOT" ls-tree "$head" -- .changes | awk 'NR == 1 { print $2 }')"
  if [ "$changes_root_type" != tree ]; then
    printf '%s\n' 'error: .changes must remain a directory holding README.md, released/, and release fragments' >&2
    exit 1
  fi

  git -C "$ROOT" ls-tree "$head" -- .changes/ >"$tmp_root/head-entries"
  awk '
    {
      mode = $1
      type = $2
      path = $0
      sub(/^[^\t]*\t/, "", path)
      name = path
      sub(/^.*\//, "", name)
      if (path == ".changes/README.md") {
        if (mode == "100644" && type == "blob") next
        print path
        next
      }
      if (path == ".changes/released") {
        if (type == "tree") next
        print path
        next
      }
      if (mode != "100644" || type != "blob") { print path; next }
      if (name !~ /^[a-z0-9][a-z0-9._-]*\.md$/) { print path; next }
    }
  ' "$tmp_root/head-entries" >"$tmp_root/invalid-entries"
  if [ -s "$tmp_root/invalid-entries" ]; then
    printf '%s\n' 'error: .changes/ accepts only README.md, released/, and release fragments named <name>.md matching ^[a-z0-9][a-z0-9._-]*\.md$ stored as regular 100644 files' >&2
    sed 's/^/  /' "$tmp_root/invalid-entries" >&2
    exit 1
  fi

  # Only an ordinary fragment change is re-rendered here: a release-seal branch
  # is already held to the stricter rendered-notes comparison above and its
  # fragments have moved into the archive, and a README-only edit carries no
  # release content to render.
  if [ "$fragment_changed" = true ] && [ "$archive_changed" = false ]; then
    head_changes="$tmp_root/head-changes"
    mkdir -p "$head_changes"
    awk '{ path = $0; sub(/^[^\t]*\t/, "", path); print path }' "$tmp_root/head-entries" |
      while IFS= read -r path; do
        case "$path" in
          .changes/released) continue ;;
        esac
        git -C "$ROOT" show "$head:$path" >"$head_changes/${path#.changes/}"
      done
    "$ROOT/scripts/release/render-release-fragments.sh" "$head_changes" >/dev/null
  fi
fi

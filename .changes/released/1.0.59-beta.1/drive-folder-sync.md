---
category: Added
---

- **Drive folder synchronization** — adds `dws drive status`, `dws drive pull`,
  `dws drive push`, and `dws drive sync` for file-level comparison and transfer
  between a local folder and a Drive folder. Differences come from exact MD5 by
  default or from modification time with `--quick`; `status` is read-only, `pull`
  and `push` are one-directional with `--if-exists skip|smart|overwrite`, and
  `sync` is bidirectional with `--on-conflict remote-wins|local-wins|keep-both|ask`.
  Only regular files are transferred — online documents and shortcuts are skipped,
  neither side deletes extra files, downloads are staged through a temporary file
  and committed with an atomic rename, and remote names that would escape
  `--local-folder` are reported as failures instead of being written. Every command
  prints a structured summary on stdout and exits non-zero when any item fails.

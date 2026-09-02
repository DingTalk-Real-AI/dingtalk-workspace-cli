---
category: Added
---

- **Drive download URL-only mode** — `dws drive download` and `dws drive download-version` accept `--url-only`, a non-downloading mode that returns the temporary signed download URL and required request headers (`downloadUrl`/`headers`, plus optional `fileName`/`fileSize`/`version`) without writing any file locally; the caller (Agent runtime / external system) performs the download itself. Signed URLs keep literal `&` separators in JSON output so they are copy-paste usable. `--url-only` is mutually exclusive with `--output`/`--overwrite`/`--part-size`/`--parallel`/`--no-resume` (explicit combinations fail fast) and stays effective through the `download --version N` compatibility routing.

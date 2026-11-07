---
category: Fixed
---

- **darwin/amd64 release asset fails to launch** (#1441) — the CGO cross-link defaulted the x86_64 deployment target to macOS 10.13, so ld64 emitted `__DATA_CONST` without `SG_READ_ONLY` and current dyld aborts the binary before `main()`. Release builds now pin `MACOSX_DEPLOYMENT_TARGET=11.0` (matching the arm64 floor), and the signed-artifact verification statically checks the `SG_READ_ONLY` flag and launches every Darwin asset it can execute before publication.

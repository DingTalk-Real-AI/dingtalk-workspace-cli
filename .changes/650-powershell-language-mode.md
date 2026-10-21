---
category: Fixed
---

- **PowerShell installer diagnostics** (#650) — `install.ps1` and `install-devapp.ps1` require FullLanguage before downloads or installation changes and report approved alternatives without closing an existing terminal. Other reported modes, including ConstrainedLanguage under WDAC audit policies, stop without method-call probes.

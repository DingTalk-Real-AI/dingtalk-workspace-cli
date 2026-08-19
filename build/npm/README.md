# DingTalk Workspace CLI npm Package

This package installs the `dws` executable and bundles the `DingTalk Workspace`
agent skills for local installation. Postinstall verifies the platform archive
and bundled Skills archive against the package's `checksums.txt` before
extracting either asset.

---
category: Added
---

- **Contact label management** — adds `dws contact label update`, `dws contact label delete`, `dws contact label add-members`, `dws contact label remove-members`, and `dws contact label update-member-scope` to modify, delete, add/remove members, and adjust member scope for contact labels (roles). Also updates `dws contact label create` to require an explicit `--type role|group`: `--type role` requires `--parent-id` with a real label group ID; `--type group` creates a root-level label group and must omit `--parent-id` (the CLI passes `parentId=-1`).
- **Contact custom field management** — adds `dws contact ext-field create`, `dws contact ext-field update`, and `dws contact ext-field delete` to manage organization custom employee fields (`add_org_ext_attrs`, `update_org_ext_attrs`, `remove_org_ext_attrs`).

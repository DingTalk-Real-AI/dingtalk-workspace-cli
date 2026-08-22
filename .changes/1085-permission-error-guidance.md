---
category: Changed
---

- **Permission error guidance and error rendering** (#1085) —
  `forbidden.accessDenied` and other permission-denied responses now exit with
  the `AUTH_PERMISSION_DENIED` code and carry apply-permission guidance
  (`dws drive permission apply-info` / `dws drive permission apply`) instead of
  a generic business-error rendering; member-validation failures such as
  "用户不存在/不属于当前组织" are classified as tool errors with a
  `--members`-with-`corpId` suggestion instead of a misleading
  resource-not-found error; business error output now surfaces the backend
  message with `code`/`logId` appended for traceability; and successful
  permission/member update/remove calls whose server body is a literal `null`
  now render `{}` so downstream JSON consumers do not fail parsing `null`.

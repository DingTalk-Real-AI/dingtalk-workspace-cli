---
category: Changed
---

- **Download host trust policy** — retires the static DingTalk/OSS download
  host allowlist, the dial-time public-IP refusal, and the IP-literal
  refusal from both the shared local download path (`drive +download`,
  `drive +version-download`, doc/minutes artifact downloads) and the chat
  message-resource path (`chat +messages-resource-download`,
  `--download-resources`). Download URLs only require HTTPS without userinfo
  and accept non-default HTTPS ports, because every dimension of a
  dedicated-deployment storage endpoint — custom domain, port, and network
  location — is decided by the customer deployment and cannot be enumerated
  or configured client-side. Verified on a dedicated deployment whose
  storage domain resolves to a customer-intranet address. Downloads align
  with the official GUI client, which applies no client-side SSRF
  interception: download URLs only ever come from authenticated service
  responses (no command accepts a user-supplied URL), TLS hostname
  verification pins the connection to the requested host, redirects are
  re-validated per hop, and service credential headers are stripped once a
  redirect leaves the original origin.
- **Upload host trust unchanged** — upload target URLs (`drive +upload`,
  minutes audio upload) keep the pre-existing public DingTalk/OSS trusted
  host requirement through a dedicated upload validator, so removing the
  download allowlist does not widen where local file bytes can be sent;
  the validator also keeps the pre-existing default-port-only HTTPS rule
  (DingTalk/OSS upload endpoints always serve on 443, so non-default ports
  accepted for dedicated-deployment downloads stay anomalous for uploads).
  Download credential headers are issued together with the download URL by
  the same authenticated service response and follow it as-is on the first
  request; redirects leaving the original host still strip them.

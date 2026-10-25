# Device flow details

Endpoints (github.com):
- `POST https://github.com/login/device/code` (client_id, scope) → device_code,
  user_code, verification_uri, expires_in (900), interval (5).
- Poll `POST https://github.com/login/oauth/access_token` with
  `grant_type=urn:ietf:params:oauth:grant-type:device_code`.

Poll responses:
| error | action |
|---|---|
| `authorization_pending` | keep polling at `interval` |
| `slow_down` | interval += 5 s |
| `expired_token` | show "Code expired", offer new code |
| `access_denied` | show "Access denied", back to Welcome |

`internal/auth.DeviceFlow` implements this over `net/http` with an injectable
clock (ADR 0005 explains why not x/oauth2). Endpoints come from `gh.Endpoints`.

UI flow: open `verification_uri` in the default browser automatically, show `user_code`
big, countdown from `expires_in`.

PAT fallback: accept classic PAT only (prefix `ghp_`); check scopes via `X-OAuth-Scopes`.
Fine-grained (`github_pat_`) → reject with explanation (Notifications API unsupported).

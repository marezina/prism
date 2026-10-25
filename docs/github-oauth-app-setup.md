# GitHub OAuth App setup (one-time, org admin)

1. GitHub → Organization settings → Developer settings → OAuth Apps → **New OAuth App**.
2. Name: `PRism`. Homepage URL: internal repo URL. Callback URL: the same URL
   (unused by device flow but required).
3. Tick **Enable Device Flow**. Register.
4. Put the **Client ID** into the release repository as an Actions *variable*
   (Settings → Secrets and variables → Actions → Variables → `PRISM_CLIENT_ID`). Release
   builds embed it as `auth.ClientID`; for local builds set `PRISM_CLIENT_ID` in the
   environment. The client ID is not secret. Do **not** create or ship a client
   secret — device flow doesn't need one.
5. Organization settings → Third-party access: if OAuth app access restrictions are on,
   approve PRism (it may appear only after the first user requests access).
6. If the org uses SAML SSO, each user authorizes the token for the org on first sign-in
   (PRism shows an "Authorize" link).

Scopes requested by PRism: `repo` (read private PRs), `notifications`, `read:org`
(org list, team review requests). PRism never writes.

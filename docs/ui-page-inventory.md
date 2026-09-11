# Management UI page inventory

This inventory treats a route as a page and also lists a state when it replaces the page's primary workspace. Confirmation clicks and transient loading or error messages are component states, not separate pages.

| # | Route | Page or primary state | Screenshot |
|---:|---|---|---|
| 1 | `/` | Signed-out login | `01-login.png` |
| 2 | `#/overview` | Overview | `02-overview.png` |
| 3 | `#/environments` | Environment inventory | `03-environments.png` |
| 4 | `#/configs` | Config inventory | `04-configs-index.png` |
| 5 | `#/configs/:config` | Config Environment contexts | `05-config-home.png` |
| 6 | `#/configs/:config/:environment` | Current raw Config | `06-config-current.png` |
| 7 | same | Revision history | `07-config-history.png` |
| 8 | same | Historical Revision inspection | `08-config-revision-v1.png` |
| 9 | same | Resolved preview | `09-config-resolved.png` |
| 10 | same | Empty comparison picker | `10-config-compare-empty.png` |
| 11 | same | Revision diff | `11-config-compare-result.png` |
| 12 | same | Merge preview | `12-config-merge-preview.png` |
| 13 | same | Replace preview | `13-config-replace-preview.png` |
| 14 | same | Clone form | `14-config-clone.png` |
| 15 | `#/vault` | Vault inventory | `15-vault-index.png` |
| 16 | `#/vault/:namespace/:item` | Vault Field metadata | `16-vault-detail.png` |
| 17 | same | Revealed Vault values | `17-vault-values.png` |
| 18 | same | Vault Revision history | `18-vault-history.png` |
| 19 | same | Structured Vault editor | `19-vault-edit.png` |
| 20 | `#/notifications` | Notification destinations | `20-notifications.png` |
| 21 | same | Notification delivery history | `21-notification-deliveries.png` |
| 22 | `#/access` | Access records | `22-access.png` |
| 23 | `#/audit` | Audit records | `23-audit.png` |
| 24 | `#/administration` | API Tokens | `24-administration-tokens.png` |
| 25 | same | Client certificates | `25-administration-certificates.png` |
| 26 | same | Embedded notification settings | `26-administration-notifications.png` |
| 27 | same | Deployment status | `27-administration-deployment.png` |
| 28 | admin-only route | Viewer forbidden state | `28-viewer-forbidden.png` |
| 29 | `#/environments` | Create Environment form | `29-environment-create.png` |
| 30 | `#/configs` | Create Config form | `30-config-create.png` |
| 31 | `#/vault` | Create Vault Item form | `31-vault-create.png` |
| 32 | `#/notifications` | Create notification destination form | `32-notification-create.png` |
| 33 | `#/administration` | Create API Token form | `33-administration-token-create.png` |
| 34 | same | Edit API Token Environment grants | `34-administration-token-grants.png` |
| 35 | same | Import client certificate form | `35-administration-certificate-import.png` |
| 36 | missing Config route | Not-found state | `36-not-found.png` |
| 37 | `#/overview` | Dark-theme Overview | `37-dark-overview.png` |
| 38 | `#/configs/:config/:environment` | Dark-theme Revision diff | `38-dark-config-diff.png` |
| 39 | `#/vault/:namespace/:item` | Dark-theme revealed Vault values | `39-dark-vault-values.png` |
| 40 | `#/environments/:environment` | Environment detail with current Config and Vault bindings | `40-environment-detail.png` |

The canonical screenshots live in `docs/ui-screenshots/` and are generated with `make local-screenshots` at a 1600 × 1000 CSS-pixel viewport. Light is the default screenshot state; the three densest representative workspaces are repeated in dark mode to verify hierarchy, syntax highlighting, diff colors, and credential-value contrast.

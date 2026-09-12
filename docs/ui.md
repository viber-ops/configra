# Management UI

The console helps operators find a resource, inspect its state and make a
controlled change. Its list/detail layout takes cues from desktop credential
managers, while retaining Configra's own Environment-based model.

## Layout and interaction

- Use a compact navigation column, searchable resource list and focused detail
  pane. Show the readable name with the immutable key, without repeating badges.
- Keep resource actions beside the resource they change. Protect dirty edits and
  retain user input after conflicts; provide the request identifier for diagnosis.
- Conceal sensitive values until explicitly requested. Changing resource identity
  must require another reveal; long fields and Environment lists need bounded views.
- Preserve keyboard navigation, visible focus, reduced-motion support and useful
  empty/error states. Narrow screens show one focused pane with a way back.
- English and Chinese share the same functionality. Light and dark themes keep
  the same hierarchy and preserve contrast in editors, diffs and credential views.

The main palette is white paper, `#f3f5f9` canvas, `#162238` ink, `#142a4a`
navigation and `#126ef2` action blue. Use system UI fonts, with monospace for keys
and source. Existing tokens live in [styles.css](../web/src/styles.css) and
[workspace.css](../web/src/workspace.css); extend them instead of adding competing
per-page themes. Typical headings are 24–28 px, controls 13–14 px, with 8 px
spacing steps and 10–12 px panel corners.

## Pages and primary states

Confirmation prompts and transient loading/error messages are component states,
not separate routes. The browser regression suite is in [web/tests/](../web/tests/).

| Route | Primary states |
| --- | --- |
| `/` | Signed-out login, language/theme choice, SSO and license links |
| `#/overview` | Observed health, configuration gaps, recent changes |
| `#/environments` | Inventory and creation |
| `#/environments/:environment` | Environment details and linked Config/Vault filters |
| `#/configs` | Inventory and creation |
| `#/configs/:config` | Environment contexts for one Config identity |
| `#/configs/:config/:environment` | Current source, history, historical inspection, explicit resolved preview, comparison picker/diff, merge/replace preview, clone and not-found states |
| `#/vault` | Item inventory and creation |
| `#/vault/:namespace/:item` | Field metadata, explicit reveal, history, structured editing and archived states |
| `#/notifications` | Destination inventory/creation and delivery history |
| `#/access`, `#/audit` | Access and Audit records, filters, request/operation correlation |
| `#/administration` | Tokens, grants, CA/client certificates, notification settings and deployment status |
| Admin-only route as Viewer | Forbidden state without administrator-only requests |

The API/storage invariants remain in the [design](design.md), [terminology](../CONTEXT.md)
and [production acceptance matrix](production-readiness.md). This page describes
presentation and navigation, not additional permission boundaries.

## Screenshots

Public screenshots use synthetic data and live in the website repository. Keep
their URLs stable; do not copy another gallery into this repository:

- [Light workspace](https://viber-ops.github.io/assets/configra/vault-light.png)
- [Dark workspace](https://viber-ops.github.io/assets/configra/vault-dark.png)
- [Mobile workspace](https://viber-ops.github.io/assets/configra/vault-mobile.png)
- [Certificate authorities](https://viber-ops.github.io/assets/configra/authorities.png)

For local review, use the prerequisites in [Try the workspace](../README.md#try-the-workspace),
but run `make local-run` from the root of this checkout, not an older release tag.
Keep it running. In a second terminal in the same directory, run `make local-seed`
to create synthetic data, then `make local-screenshots` to capture the pages at
1600 × 1000. These commands use the disposable development fixture and its
OIDC/HTTPS endpoints; do not point them at production. Captures default to
`.cache/ui-screenshots/` and must not be committed. `CONFIGRA_SCREENSHOT_DIR` can
select another reviewed output location.

Earlier UI plans and galleries remain available in
[the pre-cleanup snapshot](https://github.com/viber-ops/configra/tree/43ea1dddf86fa1dccc9a7f4b2102f4caa9c79b24/docs).

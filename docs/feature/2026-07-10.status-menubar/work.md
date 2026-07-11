# Design

## Seams

- Go status contract: domain-owned snapshot builder → stable JSON DTO. Reuse existing state, journal, overview, and attention-policy primitives; remove delivery concerns in M5.
- Swift process boundary: protocol returning command JSON / sync completion. Production `Process`; tests inject fixture responses.
- Swift presentation: decoded contract → immutable display model → `MenuBarExtra`; no domain-policy decisions.
- Platform boundaries: refresh/wake source, notification center, persistence/clock, login item each behind narrow protocols.
- Release: one app artifact added to existing release signing/notarization credentials and Homebrew publication; no second secret/config system.

## State flow

`bb status --json` + `bb overview --json` → decoder → refresh coordinator → title/menu model → SwiftUI menu. Eligible attention + Go fingerprint → notification coordinator → UserNotifications. Sync action → detached `bb sync --quiet` → completion refresh.

## Decisions

- Menu-bar-only accessory app; absence from Dock is intentional.
- Visible menu labels capped at 30 characters; full identity available in concise detail rows where practical.
- Explicit partial-source error rows; title error only when status cannot produce a valid title.
- Go contract version/shape frozen by structural golden fixtures shared directly with Swift tests.
- Notification configuration may be renamed from `notify` to attention-oriented wording during M5; breaking cleanup, no aliases/migration.
- TDD and issue boundaries decide commits; no mixed child commits.

## Validation

- Go: focused package/e2e tests, generated docs, full `just test`, `just build`, formatting/static gates discovered from CI.
- Swift: `swift test`, production build, fixture cross-decoding, injectable external-effect tests.
- macOS: project build/run entrypoint if missing; app launch/process verification; Computer Use menu/title/action/error/notification inspection; logs/screenshots under workdesk resources.
- Release: workflow/config validation, codesign/notary artifact checks where possible, Homebrew install/upgrade on local machine when safe and available.

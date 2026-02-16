# Changelog

## [0.3.0](https://github.com/niieani/bb-project/compare/v0.2.0...v0.3.0) (2026-02-16)


### Features

* **sync:** enforce catalog-scoped reconciliation and explicit clone backfill flow ([df35884](https://github.com/niieani/bb-project/commit/df358847605006bc304607a84ce43a3538a03a9c))


### Bug Fixes

* **release:** run goreleaser within release-please and support tag backfills ([3b88837](https://github.com/niieani/bb-project/commit/3b88837df3e4260617f3e98aa305c0d3ff34d083))

## [0.2.0](https://github.com/niieani/bb-project/compare/v0.1.0...v0.2.0) (2026-02-16)


### Features

* **cli:** add `bb clone`/`bb link` with explicit clone presets, GitHub HTTP repo links, and immediate state registration ([2389f5a](https://github.com/niieani/bb-project/commit/2389f5a3a498e4d3886c1ff0a2ab33d021c8f589))
* **config-wizard:** add catalog edit controls for preset/layout/push defaults and improve `bb clone` missing-arg hints ([6b0d355](https://github.com/niieani/bb-project/commit/6b0d3554cc1d0ba6448edf4befd14674d4be2bf5))
* **config:** move Lumen auto-commit default to dedicated Fixes tab and gate by availability ([098b388](https://github.com/niieani/bb-project/commit/098b38844c0d20a5fe21f0221b4007894314e974))
* **fix:** add configurable Lumen auto-commit defaults and switch visual diff shortcut to alt+v ([f5fbdd6](https://github.com/niieani/bb-project/commit/f5fbdd622158cd2e80b5492f60be854e5d81da1f))
* **lumen:** add optional Lumen integration to `bb fix` + new `bb diff`/`bb operate` passthrough commands ([5745415](https://github.com/niieani/bb-project/commit/5745415d0dd92f0752cd3891b2d0d00ce796cf9a))
* **release:** add configurable dev symlink install plus automated release-please + GoReleaser/Homebrew distribution ([7399800](https://github.com/niieani/bb-project/commit/73998002bb89be0955d9d5f407b9c3aaa3171267))


### Bug Fixes

* **fix-tui:** dynamically measure wrapped details and shrink list viewport to keep header/footer visible ([9a86687](https://github.com/niieani/bb-project/commit/9a86687e2a2a054a2a8982dc183a1826d6c893ff))
* **fix-tui:** stabilize list viewport when wrapped details change height ([176b0b8](https://github.com/niieani/bb-project/commit/176b0b82cc114fac10b23ce7cebc870357bb2146))
* **init:** clarify target resolution, skip post-init scan, and stream remote-creation stdio ([80f2acb](https://github.com/niieani/bb-project/commit/80f2acbb84a7587fc4a5de28124ba58d5dfca5d6))

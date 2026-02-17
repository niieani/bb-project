# Changelog

## [0.5.0](https://github.com/niieani/bb-project/compare/v0.4.0...v0.5.0) (2026-02-17)


### Features

* **fix:** add stash workflow, pre-push commit toggle, and commit visibility in fix summaries ([cfcd328](https://github.com/niieani/bb-project/commit/cfcd328167c3eeef9a0b02d3ef66ef551e3c1c33))
* **github:** detect and guide `gh` install/auth prerequisites across onboarding, doctor, and GitHub operations ([65fa32c](https://github.com/niieani/bb-project/commit/65fa32c3860107a4f8877636f04a72fbf56352e3))
* **repo-move:** add cross-machine catalog move flow with `bb repo move`, `catalog_mismatch`, and `bb fix move-to-catalog` ([b2c8389](https://github.com/niieani/bb-project/commit/b2c8389a9a032670d7d19fe083861315817d042e))


### Bug Fixes

* **fix-tui:** make `i` toggle ignore/unignore in `bb fix` list mode ([529d52e](https://github.com/niieani/bb-project/commit/529d52e74f3c63bb69fb1243448d0dc6898452da))
* **fix:** checkout publish branch before commit flows ([fac42e8](https://github.com/niieani/bb-project/commit/fac42e81f42cc34fb2a6741cc0da720b8732fe89))
* **fix:** scope targeted remediation, surface concrete clone failures, and add GitHub remote format templating ([c718c77](https://github.com/niieani/bb-project/commit/c718c773536ac9ba63e6927b4452e5ced5d0f481))

## [0.4.0](https://github.com/niieani/bb-project/compare/v0.3.0...v0.4.0) (2026-02-16)


### Features

* **fix-tui:** compact `bb fix` list chrome, reclaim table space, and add `not cloned` tier ([04b4e64](https://github.com/niieani/bb-project/commit/04b4e6499bc92e01daf4add5863975efa1e11a6d))
* **fix-tui:** run current fix on `enter` when no fixes are selected ([f34fadb](https://github.com/niieani/bb-project/commit/f34fadb188bd9bdc18d787ec2a0386bca0507cc9))
* **fix-tui:** standardize bordered list header pattern and tighten footer/details rendering ([86f1c1e](https://github.com/niieani/bb-project/commit/86f1c1e5ed866046ebde56ac84064cdf84ede766))
* **fix:** probe GitHub push access via `gh`, refresh unknown access in `bb fix`, and block push fixes when access is unknown ([8db4beb](https://github.com/niieani/bb-project/commit/8db4beb457d4f690660e0c5de941e2077a1b3cc1))


### Bug Fixes

* **fix-tui:** keep list panel top-anchored by removing render-time top padding ([24264d9](https://github.com/niieani/bb-project/commit/24264d9bae5063c7bcfe36f158bad25bb84051aa))
* **fix-tui:** keep responsive summary fallback visually consistent with pill stats ([104e778](https://github.com/niieani/bb-project/commit/104e778d539b6da8ec1aaccdefcf54f99f8d4b36))
* **fix-tui:** keep titled top border color in sync during `r` revalidation ([3fec908](https://github.com/niieani/bb-project/commit/3fec908cc4b8476a22a0d7303466f3160729c330))
* **fix-tui:** make list-mode stats responsive on narrow widths to prevent broken pill layout ([0e47881](https://github.com/niieani/bb-project/commit/0e478819ca80202e3677a1bdd78c2b3620f30bb1))
* **fix-tui:** remove inter-panel blank rows by expanding list viewport to fill available height ([43c67f1](https://github.com/niieani/bb-project/commit/43c67f13a63a9e582c1648111373d6fba23034b5))
* **fix-tui:** restore sticky footer while keeping top anchor stable when details wrap ([97f98d9](https://github.com/niieani/bb-project/commit/97f98d9d1666a5586eb322bfbec73a8890d49396))
* **fix-tui:** stabilize list-mode layout under wrapped details without scroll jumps ([8571691](https://github.com/niieani/bb-project/commit/8571691017c4bd39992f9f47fc4bf440f92a20d7))
* **fix-tui:** stop single-step list jumps when wrapped details change by removing selection-driven viewport re-bucketing ([d8ad90c](https://github.com/niieani/bb-project/commit/d8ad90c2c3c70efb578872d12868271b9211c87e))
* **fix-ui:** normalize interactive `bb fix` startup progress and hide noisy push-probe error dumps ([a3d5fe1](https://github.com/niieani/bb-project/commit/a3d5fe15e220ef3977ff3c005901192e378a0259))
* **push-access:** trust GitHub viewerPermission and skip dry-run when GH already knows access ([32b9ee6](https://github.com/niieani/bb-project/commit/32b9ee66260f07f016fc2e31363ebe5332f4572b))
* **scan:** re-probe unknown push_access during metadata load for scan ([1753817](https://github.com/niieani/bb-project/commit/1753817be3bade4e823d103062c5bab863f7af74))

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

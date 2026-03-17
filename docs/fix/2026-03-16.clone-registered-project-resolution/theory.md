# Registered Project Clone Resolution

## Problem Thesis

`bb clone` currently assumes its argument is always a remote repository spec. That forces catalog selection through `--catalog` or `clone.default_catalog`, even when the repo is already registered in shared metadata with a canonical `repo_key`, origin URL, and catalog. The result is a mismatch between the shared state model and the command UX: users can refer to an existing project, but `clone` ignores the metadata that already defines where that project belongs.

## Operating Theory

The codebase already has the pieces needed for better behavior. Shared repo metadata stores the authoritative `repo_key`, `origin_url`, and preferred catalog, while local selector code already resolves `repo_key` and unique names for commands like `bb link`. The gap is specific to `bb clone`: it parses raw remote input first and only then asks for a catalog, so bare registered names fail fast and `owner/repo` input cannot reuse the catalog recorded in metadata.

The right lever is to let `bb clone` consult repo metadata before falling back to raw remote parsing. When metadata matches, the command should clone into the catalog and relative path defined by the registered project instead of asking the user to restate that information locally.

## Systematic Strategy

Keep the current remote clone flow intact for unregistered repos. Add a metadata-aware resolution step ahead of it:

1. Resolve the input against repo metadata using registered selectors and origin matching.
2. When a metadata match exists, take the origin URL and target catalog/path from metadata.
3. Only use `clone.default_catalog` and path derivation rules when no registered project match exists.

This keeps the behavior additive for raw remotes while making registered projects converge on their shared identity.

## Current Decision

An explicit `--catalog` is rejected when it conflicts with a registered repo's canonical catalog. That keeps clone behavior aligned with the rest of the system, where the shared `repo_key` catalog is authoritative and cross-catalog fallback is intentionally avoided.

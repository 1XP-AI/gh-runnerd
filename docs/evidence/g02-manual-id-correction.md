# G02 manual App ID correction

This addresses the reproduced P2 [PR26 finding](https://github.com/1XP-AI/gh-runnerd/pull/26#discussion_r3949159062), tracked by [issue #30](https://github.com/1XP-AI/gh-runnerd/issues/30), with enrollment context in [issue #2](https://github.com/1XP-AI/gh-runnerd/issues/2). It changes only the private journal's handling of an unverified manual App ID. No App, installation, token, credential store or runtime was contacted or changed.

The finding was posted against original commit `ec51a1991c13daffbdc74adbaad279e181b1ad2a` after PR26 merged. Its original-commit staleness did not establish a fix: the relevant code still reproduced on audited main `f1fe70c2730e266e372bd206aa83f4c43e55ae5a`. At audit time GitHub reported the thread unresolved and not outdated.

## Recovery boundary

Previously, a first manual attempt wrote its supplied App ID into `prepared` before reading the PEM or authenticating the App. An incorrect ID then prevented the corrected ID from reaching verification through the same journal. An invalid PEM with the same correct App ID already recovered successfully; the defect was the unverified incorrect ID.

New `prepared` records retain the approved owner, App name and organization identities while leaving App ID unset. For an existing `prepared` record, including one containing a legacy unverified ID, opening for manual correction preserves the old bytes. The existing `VerifyManual` flow parses the key and authenticates the exact App ID, slug and organization owner before its durable `verifying` transition pins that ID. No extra API call or authority is introduced.

Known IDs in every later phase stay immutable: `registration_started`, `conversion_started`, `conversion_failed`, `app_received`, `verifying`, `verification_failed` and `verified`. A failure after the authenticated pin cannot reopen correction. Unknown IDs from an interrupted Manifest retain the existing operator-driven reconciliation behavior. The presence of any journal still blocks a new Manifest registration; recovery never deletes or resets it. Owner, name and organization identities remain fixed in all phases.

## Synthetic TDD evidence

Red commit `2e84bf8` fails before the production fix. `TestManualIdentityCorrectionBeforeAuthentication` reproduces first-attempt valid-key/wrong-ID, invalid-key/wrong-ID and legacy-`prepared` cases. Each reports `phase=prepared app_id=72 new_identity_checks=0 verified_organizations=0` on the corrected ID71 attempt. `TestManualIdentityIsUnpinnedUntilAuthenticated` independently observes the premature ID at the fake authentication callback. The failures are behavioral assertions, with no race-detector failure.

The focused tests also cover:

- Same-correct-ID invalid-to-valid PEM recovery.
- Exact existing record bytes retained until corrected identity authentication.
- A durable authenticated ID before organization verification, retained when that verification fails.
- Different-ID refusal before API use for every later known-ID phase.
- A synthetic leftover `record.next` causing a later journal write to fail, while the authenticated `verifying` ID remains locked and its bytes unchanged.
- Fixed owner/name/organization identity and the existing Manifest restart refusal.
- No PEM or key marker written to journal files.

All new tests use freshly generated synthetic RSA material and injected in-memory API responses. They perform no network requests. Go `1.26.8` fresh module race tests and vet passed after the fix; these checks cover the module and enrollment command. `GOTOOLCHAIN=go1.26.8 make check` also passed, including both offline modules and the configured root vulnerability scan. The root bootstrap reported its existing explicit fuzz skip because no fuzz targets are present. Exact-head independent and external review remain required before integration. No live GitHub recovery, power-loss durability or production credential-storage claim follows from these synthetic results.

# G01 concurrent admission initialization correction

Issue: #1. This corrects initial claim creation in the finite controller and
worker experiments; it does not release their permanent capacity reservations.

Two first-time processes could interleave exclusive file creation and the file
lock. The second process could lock the still-empty claim before the creator,
then reject its empty contents while the creator also failed its lock. Both
would refuse and leave an unusable empty permanent claim. The existing worker
concurrency test reproduced zero creates in 3 of 20 race-enabled runs on the
reviewed main commit `c1c0b6a`.

Both initializers now take a short nonblocking lock on the already validated
admission directory before opening or creating the claim. They retain this
initialization lock through claim validation, the existing lifetime claim lock,
and file/directory durability checks. Closing the temporary directory handle
releases only the initialization lock. The claim lock, exact identity checks,
unknown-state refusal, and permanent capacity pin remain unchanged.

## TDD and validation

- Red `7479f7f`: holding the real directory lock still admitted a contender and
  created a claim in both packages. Both deterministic tests failed as intended.
- Green: both directory-lock tests and the existing worker concurrent-first-start
  test passed 50 repetitions under the race detector. The new controller
  concurrent-first-start positive control also passed 50 repetitions, admitting
  exactly one synthetic create per run.
- All effects in these tests use fake APIs/runtimes and private temporary roots.
  No live admission root, GitHub resource, runner or Docker runtime was changed.
- Full public validation and independent/external review results are recorded in
  the linked correction PR. These tests do not establish live gate completion.

Malformed or empty claims left by an earlier failure remain refused. This change
adds no reset, adoption, retry of unknown creation, or capacity release path.

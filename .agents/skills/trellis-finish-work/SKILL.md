---
name: trellis-finish-work
description: "Wrap up the current session: verify quality gate passed, remind user to commit, archive completed tasks, and record session progress to the developer journal. Use when done coding and ready to end the session."
---

# Finish Work

Wrap up the current session and preserve project-useful knowledge before archiving.

## Step 1: Quality Gate

`trellis-check` should have already run in Phase 3. For implementation changes, complete the applicable lint, type-check, tests and spec checks. For documentation-only changes, verify factual evidence, source links, task coverage and consistency; do not claim product tests were run.

## Step 2: Capture the Archive Retrospective (required)

Follow [Archive retrospective guide](../../../.trellis/spec/guides/archive-retrospective-guide.md) before every archive, including direct archive requests and bulk cleanup.

- Read the PRD, research/decisions and real verification records. Record actual delivery, reusable decisions and reasons, pitfalls/boundaries, validation evidence and limitations, unfinished work/next steps, and source/spec paths.
- Write `<task>/summary.md`. A batch may use `archive/YYYY-MM/ARCHIVE-SUMMARY.md` if it covers every affected task with useful conclusions, evidence and remaining work.
- Apply `trellis-update-spec` to promote durable contracts into existing specs; link them instead of maintaining duplicate rules. If there is no new reusable lesson, record that judgment and the existing rule.
- Preserve the pre-archive state. Separate implemented behavior, historical local verification, target acceptance and planning. `task.py archive` automatically writes `completed`; it is not proof that a feature or an acceptance gate passed.
- Keep evidence paths valid after moving tasks and link consolidated lessons from the spec guide index. Exclude secrets and private fixture values.

## Step 3: Remind User to Commit

If there are uncommitted changes, remind the user to review and commit when ready. Do NOT run `git commit` — the human commits after testing.

## Step 4: Archive and Record the Session

Normally archive after the user's commit. If the user explicitly requests archiving first, complete that request after the retrospective, preserving unverified and unfinished work in the summary.

```text
python -B scripts/project_env.py -- python -B .trellis/scripts/task.py archive <task-name> --no-commit
python -B scripts/project_env.py -- python -B .trellis/scripts/task.py list
```

Use `--no-commit` to prevent Trellis from committing unrelated staged work. Verify task materials, summary links, expected archive count and the current-task pointer; leave unrelated active tasks untouched.

Record the session with actual commit hashes when available, or leave the default uncommitted marker rather than inventing one:

```text
python -B scripts/project_env.py -- python -B .trellis/scripts/add_session.py --title "Session Title" --commit "hash1,hash2" --summary "Useful lessons, evidence, limitations and archive summary path" --no-commit
```

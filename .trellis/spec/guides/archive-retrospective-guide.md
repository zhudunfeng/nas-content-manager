# Archive Retrospective Guide

Use this checklist before every Trellis archive, including bulk cleanup and explicit requests to close unfinished work. The goal is to preserve knowledge that changes the next developer's decisions.

## Before moving tasks

- Read the task PRD, decisions/research, implementation notes and actual verification records. Do not infer delivery from checkboxes or `task.json.status` alone.
- Record the pre-archive lifecycle state and separate implemented behavior, historical local checks, target-device acceptance, unverified assumptions and planned work. The archive CLI writes `completed` and `completedAt` automatically.
- Write `<task>/summary.md`. For a batch, a monthly `archive/YYYY-MM/ARCHIVE-SUMMARY.md` may replace individual summaries if it maps every task to useful conclusions, evidence and remaining work.
- Capture actual delivery; reusable decisions and why; pitfalls and boundaries; verification commands/results/limitations; unfinished work and next steps; source paths and relevant spec links. If there is no new reusable lesson, say why and link the existing rule.
- Promote durable implementation contracts to the relevant [backend](../backend/index.md) or [frontend](../frontend/index.md) spec. Keep this guide a checklist and retain detailed evidence in the task/archive.
- Give every conclusion a source. Label a historical test by date/environment; a successful fixture or cross-build does not prove live-site or target NAS acceptance. Keep secrets and private fixture values out of summaries.

## Archive and verify

```text
python -B scripts/project_env.py -- python -B .trellis/scripts/task.py archive <task-name> --no-commit
python -B scripts/project_env.py -- python -B .trellis/scripts/task.py list
```

- Check the destination for collisions before moving. Preserve task materials and existing archived files.
- Check that source links still resolve after the move. Use a spec/index entry to make consolidated lessons discoverable.
- Verify the expected tasks are archived, summaries are present, and the current-task pointer is cleared when its task is archived. Preserve unrelated active tasks and user edits.
- Report the useful lessons and unresolved acceptance work, as well as the archive result. Leave Git commits to the user.

## Useful starting points

- [November 2026 archive summary](../../tasks/archive/2026-11/ARCHIVE-SUMMARY.md) — retrospective for the 50-task batch of 2026-11-27; preserves the distinction between archive status and delivery evidence.
- [Regression baseline contract](../backend/baseline-contract.md) — evidence kinds and truthful target acceptance.
- [Development workflow](../../workflow.md) — lifecycle and spec-update requirements.

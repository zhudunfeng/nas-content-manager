# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

The first release is a standard-library Go HTTP service with pure-Go SQLite. Its executable contracts are documented below; older topic templates remain marked as templates until expanded.

## Pre-Development Checklist

1. Read [Runtime contract](./runtime-contract.md), `docs/api.md`, and the current task PRD.
2. Inspect the affected handler and its storage/path helpers before changing behavior.
3. Keep Chinese function/field/config comments, explicit types, and Chinese business logs.
4. For downloads or scan/read/storage changes, also read [Download contract](./download-contract.md).
5. For source search, secret configuration or search-origin additions, also read [Torrent search contract](./torrent-search-contract.md).
6. For engine setup, helper/Docker integration or managed connection changes, read [Engine bootstrap contract](./engine-bootstrap-contract.md).
7. For native launchers, lifecycle, tool resolution, embedded assets or packaging, read [Cross-platform contract](./cross-platform-contract.md).
8. For RSS, collection tasks, worker adapters or collection publication, read [Collection contract](./collection-contract.md).
9. For MoviePilot/AutoBangumi/Sonarr/Radarr installation, read [Component deployment contract](./collection-component-contract.md).

## Quality Check

Run `go vet ./...`, `go test ./...`, `go build ./cmd/nas`, and the isolated `scripts/verify.py` workflow when API/storage/media behavior changes. See the runtime contract for boundary assertions. Remove only newly generated temporary unit-test files after execution per AGENTS.md; preserve the reusable integration harness.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Runtime contract](./runtime-contract.md) | Actual API, SQLite, jobs, path boundaries, streaming and resource rules | Implemented |
| [Download contract](./download-contract.md) | Optional qB coordinator, v2 migration, selected publication gate and deployment boundaries | Implemented |
| [Torrent search contract](./torrent-search-contract.md) | Optional bounded Torznab aggregation, private result cache and original managed-download handoff | Implemented |
| [Engine bootstrap contract](./engine-bootstrap-contract.md) | Optional private helper, fixed Docker templates, durable recovery and managed runtime activation | Implemented |
| [Cross-platform contract](./cross-platform-contract.md) | Shared lifecycle, Windows launcher, embedded assets, immutable desktop deployments and releases | Implemented |
| [Collection contract](./collection-contract.md) | Multi-source collection, durable worker, publication and exact import coverage | Implemented |
| [External collection contract](./external-collection-contract.md) | Schema v4, typed external automation, durable intents and MoviePilot workflows | Implemented |
| [Component deployment contract](./collection-component-contract.md) | Fixed Docker components, private volume ownership, persistent deployment and desktop v3 | Implemented |
| [Directory Structure](./directory-structure.md) | Module organization and file layout | To fill |
| [Database Guidelines](./database-guidelines.md) | ORM patterns, queries, migrations | To fill |
| [Error Handling](./error-handling.md) | Error types, handling strategies | To fill |
| [Quality Guidelines](./quality-guidelines.md) | Code standards, forbidden patterns | To fill |
| [Logging Guidelines](./logging-guidelines.md) | Structured logging, log levels | To fill |

---

## How to Fill These Guidelines

For each guideline file:

1. Document your project's **actual conventions** (not ideals)
2. Include **code examples** from your codebase
3. List **forbidden patterns** and why
4. Add **common mistakes** your team has made

The goal is to help AI assistants and new team members understand how YOUR project works.

---

**Language**: All documentation should be written in **English**.

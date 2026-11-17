# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

The first release is a standard-library Go HTTP service with pure-Go SQLite. Its executable contracts are documented below; older topic templates remain marked as templates until expanded.

## Pre-Development Checklist

1. Read [Runtime contract](./runtime-contract.md), `docs/api.md`, and the current task PRD.
2. Inspect the affected handler and its storage/path helpers before changing behavior.
3. Keep Chinese function/field/config comments, explicit types, and Chinese business logs.

## Quality Check

Run `go vet ./...`, `go test ./...`, `go build ./cmd/nas`, and the isolated `scripts/verify.py` workflow when API/storage/media behavior changes. See the runtime contract for boundary assertions. Remove only newly generated temporary unit-test files after execution per AGENTS.md; preserve the reusable integration harness.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Runtime contract](./runtime-contract.md) | Actual API, SQLite, jobs, path boundaries, streaming and resource rules | Implemented |
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

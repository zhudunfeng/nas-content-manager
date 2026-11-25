# Frontend Development Guidelines

> Best practices for frontend development in this project.

---

## Overview

The first release uses Vue 3 SFCs, strict TypeScript, Vite, and local CSS/SVG. The concrete application contract below takes precedence over the older unfilled topic templates.

## Pre-Development Checklist

1. Read [Application contract](./app-contract.md), `docs/api.md`, and the task's visual design.
2. Inspect actual neighboring components, shared types, and API helpers.
3. Preserve Chinese method/field/config comments, explicit types, and accessible controls.
4. For the JMComic search/download dialog and its source form, also read [JMComic plugin contract](../backend/jmcomic-contract.md); preserve stable submission IDs and metadata invalidation.
5. For UAV video search/download, also read [UAV plugin contract](../backend/uav-contract.md); preserve four-site selection, single-episode IDs, expiring metadata and stable submission UUIDs.

## Quality Check

Run `npm --prefix web run typecheck` and `npm --prefix web run build`. For UI/reader changes, check real endpoints in a browser at 375/768/1024/1440 widths, both themes, error/empty states, keyboard/dialog behavior, and reader progress. See `docs/verification.md` for actual executed coverage.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Application contract](./app-contract.md) | Actual state, API, readers, safety, responsive interaction | Implemented |
| [Directory Structure](./directory-structure.md) | Module organization and file layout | To fill |
| [Component Guidelines](./component-guidelines.md) | Component patterns, props, composition | To fill |
| [Hook Guidelines](./hook-guidelines.md) | Custom hooks, data fetching patterns | To fill |
| [State Management](./state-management.md) | Local state, global state, server state | To fill |
| [Quality Guidelines](./quality-guidelines.md) | Code standards, forbidden patterns | To fill |
| [Type Safety](./type-safety.md) | Type patterns, validation | To fill |

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

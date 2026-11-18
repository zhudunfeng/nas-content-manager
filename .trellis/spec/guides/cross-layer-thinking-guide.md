# Cross-Layer Thinking Guide

> **Purpose**: Think through data flow across layers before implementing.

---

## The Problem

**Most bugs happen at layer boundaries**, not within layers.

Common cross-layer bugs:
- API returns format A, frontend expects format B
- Database stores X, service transforms to Y, but loses data
- Multiple layers implement the same logic differently

---

## Before Implementing Cross-Layer Features

### Step 1: Map the Data Flow

Draw out how data moves:

```
Source → Transform → Store → Retrieve → Transform → Display
```

For each arrow, ask:
- What format is the data in?
- What could go wrong?
- Who is responsible for validation?

### Step 2: Identify Boundaries

| Boundary | Common Issues |
|----------|---------------|
| API ↔ Service | Type mismatches, missing fields |
| Service ↔ Database | Format conversions, null handling |
| Backend ↔ Frontend | Serialization, date formats |
| Component ↔ Component | Props shape changes |

### Step 3: Define Contracts

For each boundary:
- What is the exact input format?
- What is the exact output format?
- What errors can occur?

---

## Common Cross-Layer Mistakes

### Mistake 1: Implicit Format Assumptions

**Bad**: Assuming date format without checking

**Good**: Explicit format conversion at boundaries

### Mistake 2: Scattered Validation

**Bad**: Validating the same thing in multiple layers

**Good**: Validate once at the entry point

### Mistake 3: Leaky Abstractions

**Bad**: Component knows about database schema

**Good**: Each layer only knows its neighbors

---

## Checklist for Cross-Layer Features

Before implementation:
- [ ] Mapped the complete data flow
- [ ] Identified all layer boundaries
- [ ] Defined format at each boundary
- [ ] Decided where validation happens

After implementation:
- [ ] Tested with edge cases (null, empty, invalid)
- [ ] Verified error handling at each boundary
- [ ] Checked data survives round-trip

---

## When to Create Flow Documentation

Create detailed flow docs when:
- Feature spans 3+ layers
- Multiple teams are involved
- Data format is complex
- Feature has caused bugs before

## External Service Round Trips

For an authenticated service adapter, verify the complete exchange with the actual client and a real isolated service: login status/body/cookie names, a subsequent authenticated call, expired-session refresh, and failure propagation. A separate probe using a cookie jar can pass while a production client that manually forwards a hardcoded cookie name fails. Mocks must follow observed version-specific responses rather than validate the adapter's assumptions.

Record only sanitized protocol metadata in evidence. Do not log cookie values, credentials, private source URLs or unfiltered upstream response bodies. Keep a reusable end-to-end probe when temporary unit tests must be removed.

When a network response changes persisted state, also verify revision ownership, transaction rollback and later reads. A conditional update that affects zero rows must not leave dependent manifest rows or delete recovery input. Publication checks must run before every early-return path that could leave previously ready data readable.

For short-lived external search results that create durable tasks, check whether an accepted retry survives cache expiry/restart, whether identity aliases preserve the same ownership, and whether cancellation or partial source failure is represented correctly. See the [torrent search contract](../backend/torrent-search-contract.md) for concrete budgets, credential boundaries and request binding.

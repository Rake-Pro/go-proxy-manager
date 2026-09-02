<!--
RUNBOOK TEMPLATE - copy this file to start a new runbook / how-to, then delete
these comments. House style for operational docs in this project:

- One action per step. Step titles are imperative: "Snapshot the data", not
  "Snapshotting".
- Every command is copy-paste-ready. Every placeholder is <UPPER_SNAKE> and is
  defined once in the Variables table - no unexplained blanks.
- Each step states what success looks like (Expect) and gives an explicit check
  (Verify) with its expected output. Add "If it fails" only where there is a
  known trap.
- Flag the disruptive/irreversible step and any decision point as a GATE callout
  so it cannot be skimmed past.
- Present tense, active voice, sentence case. Say what a command does, not how
  it is built.
- Keep validation as a checklist the reader ticks; keep a Rollback and a
  "Done when".
-->

# Runbook: <TITLE - imperative, e.g. "Migrate X from A to B">

**Purpose:** <one sentence: what this accomplishes and the safety stance>.
**Audience:** <who runs this, and the access they need>.
**Time:** <rough estimate>.
**Risk:** <where it is safe; name the one disruptive step and that it is reversible>.

## Prerequisites

- [ ] <access / credential / tool that must already work>
- [ ] <thing to confirm before starting>

## Variables

| Placeholder    | Meaning                          | Value / example        |
| -------------- | -------------------------------- | ---------------------- |
| `<PLACEHOLDER>`| <what it is>                     | <example>              |

---

## Step 1 - <imperative title>

Why: <one line, only if non-obvious>.

Run:
```
<command>
```

Expect: <what state/output success looks like>.

Verify:
```
<a check command>
```
-> <expected output>.

If it fails (`<symptom>`): <cause -> fix>.   <!-- include only where there's a known trap -->

---

## Step 2 - <imperative title>

<...same shape...>

---

## Step N - GATE: <what must be true to proceed>   <!-- use for decision points / before disruptive steps -->

> **<Action> is blocked until:**
> 1. <condition>, and
> 2. <condition>.
>
> If either is incomplete, stop here.

---

## Step N+1 - <the disruptive step>

<...Run / Expect / Verify...>

---

## Rollback

<how to revert to the prior known-good state, and why it is safe>.

## Done when

- <completion criterion>
- <completion criterion>

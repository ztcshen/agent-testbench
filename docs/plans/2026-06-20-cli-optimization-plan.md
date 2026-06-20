# CLI Optimization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the AgentTestBench CLI easier for agents and humans to navigate by shrinking the default surface, adding task-intent discovery, and grouping map commands by lifecycle.

**Architecture:** Keep existing commands callable. Add metadata, help grouping, and task-intent wrappers first; only demote commands from daily visibility after replacement paths are visible and tested. The command catalog remains the source of truth for help, discovery, tiers, tags, and replacement hints.

**Tech Stack:** Go CLI in `cmd/agent-testbench`, existing Store-backed task records in `internal/store`, command catalog tests in `cmd/agent-testbench/command_catalog_test.go`, task tests in `cmd/agent-testbench/task_commands_test.go`.

---

## Current Baseline

- `agent-testbench commands --all --json`: 160 commands.
- Tiers: 40 daily, 104 advanced, 16 compat.
- Largest areas: case 23, environment 21, map 19, profile 17, workflow 16.
- The first implementation should not delete commands. It should improve the recommended path and keep compatibility stable.

## Follow-Up Baseline: Canonicalize Duplicate Entrypoints

The first implementation reduced the default surface but left duplicate command
paths callable. The follow-up goal is intentionally destructive: remove
duplicate public CLI paths where one canonical command already covers the same
capability. This follows the same shape as mature CLIs that prefer one
object-oriented command path plus grouped help over many parallel aliases.

- Baseline before this slice: `commands --all --json` exposed 163 commands.
- Target after this slice: remove duplicate public entrypoints while preserving
  the underlying execution capability through canonical commands.
- Canonical substitutions:
  - `case suite <view>` -> `case suite report --view <view>`
  - `workflow acceptance start/report` -> `environment acceptance start/report`
  - `baseline get/set` -> `gate baseline get/set`
  - `map run explain` -> `map plan inspect`
  - top-level `watch` -> `task watch`
  - `template-packages verify` -> `template-package verify`
- Verification:
  - Add a regression test proving removed duplicate entrypoints no longer appear
    in `commands --all --json`.
  - Add a regression test proving removed root aliases fail as unknown commands.
  - Update existing behavior tests to use the canonical commands.
  - Run `go test ./...`, `make lint`, `make quality`, and duplicate-code scan.

## Slice Group 1: Shrink The Default Entry Surface

### Task 1: Document Daily Command Admission Rules

**Files:**
- Modify: `docs/quickstart.md`
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Add a short daily/advanced/compat rule section to `docs/quickstart.md`.
2. Add `commandCatalogDailyAdmissionReason(command string) string` in `cmd/agent-testbench/command_catalog.go`.
3. Include the reason in `commandCatalogItem` JSON as `dailyReason,omitempty`.
4. Add a failing test that `commands --json` explains at least `map run`, `case run`, and `workflow report`.
5. Implement the minimal catalog field.
6. Run: `go test ./cmd/agent-testbench -run TestCommands -count=1`.
7. Commit: `docs: define cli command tier rules`.

**Acceptance:**
- A daily command can explain why it is daily.
- Advanced and compat commands remain callable.

### Task 2: Add Replacement Coverage For Non-Daily Workflow Commands

**Files:**
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Write a failing test that every `workflow` command with tier `advanced` or `compat` has a non-empty `replacement` or `nextAction`.
2. Add replacement hints toward `map explain`, `map run`, `map gate`, or `map atlas` where applicable.
3. Run: `go test ./cmd/agent-testbench -run TestCommands -count=1`.
4. Commit: `cli: add workflow replacement hints`.

**Acceptance:**
- Agents can discover map-first alternatives for non-daily workflow commands.

### Task 3: Reduce Daily Surface To The First Target Set

**Files:**
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Write a failing test that default daily count is at or below 30.
2. Demote low-frequency daily commands to advanced:
   - `environment acceptance start`
   - `environment acceptance report`
   - `workflow report`
   - `case suite inspect`
   - `case suite plan`
   - selected `gate baseline` entries if replacement hints are ready
3. Keep all demoted commands visible through `commands --all`.
4. Run: `go test ./cmd/agent-testbench -run TestCommandsDefaultSurfaceShowsDailyCommandsOnly -count=1`.
5. Commit: `cli: narrow default daily command surface`.

**Acceptance:**
- `commands --json` is smaller.
- `commands --all --json` still contains all old command paths.

### Task 4: Make Top-Level Help Recommend Workflows Instead Of Listing Everything

**Files:**
- Modify: `cmd/agent-testbench/help.go`
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Write a failing test that top-level help contains "Recommended workflows" and does not list every advanced command family.
2. Keep only core daily commands and examples in `help.go`.
3. Add an explicit line pointing to `agent-testbench commands --all`.
4. Run: `go test ./cmd/agent-testbench -run TestTopLevelHelpShowsStoreFlagNotLegacyStoreURL -count=1`.
5. Commit: `cli: make top-level help task-oriented`.

**Acceptance:**
- `agent-testbench --help` is a start page, not a full manual.

## Slice Group 2: Add A Task-Intent Layer

### Task 5: Add Built-In Task Catalog

**Files:**
- Modify: `cmd/agent-testbench/task_commands.go`
- Create: `cmd/agent-testbench/task_catalog.go`
- Test: `cmd/agent-testbench/task_commands_test.go`

**Steps:**
1. Write a failing test for `agent-testbench task catalog --json`.
2. Define built-in task descriptors:
   - `map-maintain`
   - `map-execute`
   - `environment-restore`
   - `case-diagnose`
3. Each descriptor should expose goal, required inputs, steps, and recommended commands.
4. Run: `go test ./cmd/agent-testbench -run TestTask -count=1`.
5. Commit: `task: add built-in task catalog`.

**Acceptance:**
- `task catalog --json` returns stable machine-readable task definitions.

### Task 6: Add `task suggest`

**Files:**
- Modify: `cmd/agent-testbench/task_commands.go`
- Modify: `cmd/agent-testbench/task_catalog.go`
- Test: `cmd/agent-testbench/task_commands_test.go`

**Steps:**
1. Write a failing test for `task suggest --goal "maintain map" --json`.
2. Match goal text against task id, name, tags, and command tags.
3. Return ranked suggestions with `reason`.
4. Run: `go test ./cmd/agent-testbench -run TestTaskSuggest -count=1`.
5. Commit: `task: suggest tasks from user goals`.

**Acceptance:**
- `task suggest --goal "maintain map"` returns `map-maintain`.
- `task suggest --goal "execute map"` returns `map-execute`.

### Task 7: Add `task plan`

**Files:**
- Modify: `cmd/agent-testbench/task_commands.go`
- Modify: `cmd/agent-testbench/task_catalog.go`
- Test: `cmd/agent-testbench/task_commands_test.go`

**Steps:**
1. Write a failing test for `task plan map-maintain --map ID --json`.
2. Render command steps without executing them.
3. Include missing-input diagnostics for required inputs.
4. Run: `go test ./cmd/agent-testbench -run TestTaskPlan -count=1`.
5. Commit: `task: plan built-in task workflows`.

**Acceptance:**
- `task plan map-maintain --map ID` shows the sequence before execution.
- Missing `--map` returns a clear error.

### Task 8: Implement Read-Only `task run map-maintain`

**Files:**
- Modify: `cmd/agent-testbench/task_commands.go`
- Modify: `cmd/agent-testbench/task_catalog.go`
- Test: `cmd/agent-testbench/task_commands_test.go`

**Steps:**
1. Write a failing test that `task run map-maintain --map ID --dry-run --json` returns planned subcommands.
2. Support read-only execution for `map doctor` and `map coverage`.
3. Do not call `map publish` in this slice.
4. Persist or report step results using existing task run output shape.
5. Run: `go test ./cmd/agent-testbench -run TestTaskRunMapMaintain -count=1`.
6. Commit: `task: run read-only map maintenance`.

**Acceptance:**
- The task can be used safely before destructive or publishing steps exist.

### Task 9: Implement `task run map-execute`

**Files:**
- Modify: `cmd/agent-testbench/task_commands.go`
- Modify: `cmd/agent-testbench/task_catalog.go`
- Test: `cmd/agent-testbench/task_commands_test.go`

**Steps:**
1. Write a failing test for `task run map-execute --map ID --dry-run --json`.
2. Plan the sequence: `map explain --save`, `map run`, `map gate`, `map atlas`.
3. Add `--execute` or `--dry-run=false` only after dry-run is stable.
4. Run focused tests.
5. Commit: `task: plan map execution workflow`.

**Acceptance:**
- Agents can ask for one task and see the map execution lifecycle.

## Slice Group 3: Reorganize Map Commands By Lifecycle

### Task 10: Add Map Lifecycle Metadata

**Files:**
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Write a failing test that map commands expose `lifecycle` metadata.
2. Add lifecycle values:
   - `inspect`: `map list`, `map workflows`, `map coverage`, `map plans`
   - `maintain`: `map doctor`, `map diff`, `map validation list`, `map validation attach`, `map update`, `map snapshot`, `map publish`
   - `plan`: `map explain`, `map plan inspect`, `map run explain`
   - `execute`: `map run`, `map gate`
   - `review`: `map atlas`
3. Run: `go test ./cmd/agent-testbench -run TestCommands -count=1`.
4. Commit: `map: add command lifecycle metadata`.

**Acceptance:**
- `commands --area map --all --json` exposes lifecycle for each map command.

### Task 11: Group `map --help` By Lifecycle

**Files:**
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Write a failing test that `map --help` contains lifecycle headings.
2. Render grouped help when the prefix is exactly `map`.
3. Preserve exact command help for `map run --help`.
4. Run: `go test ./cmd/agent-testbench -run TestGroupedHelpShowsAreaAndExactCommandUsage -count=1`.
5. Commit: `map: group help by lifecycle`.

**Acceptance:**
- `map --help` is not a flat list of 19 commands.

### Task 12: Sort Task-Oriented Command Search By Recommended Order

**Files:**
- Modify: `cmd/agent-testbench/command_catalog.go`
- Test: `cmd/agent-testbench/command_catalog_test.go`

**Steps:**
1. Write a failing test for `commands --filter "maintain map" --all --json`.
2. Ensure output order is `map doctor`, `map coverage`, `map diff`, `map validation list`, `map validation attach`, then publish/update commands.
3. Add a stable rank field or sort rule.
4. Run: `go test ./cmd/agent-testbench -run TestCommandsSupportTaskOrientedFilters -count=1`.
5. Commit: `cli: sort task-oriented command search`.

**Acceptance:**
- Search results become a usable next-step sequence.

### Task 13: Optional Lifecycle Alias Planning

**Files:**
- Modify: `cmd/agent-testbench/map_commands.go`
- Test: `cmd/agent-testbench/map_commands_test.go`

**Steps:**
1. Write a failing test for `map maintain --map ID --plan --json`.
2. Implement it as a read-only alias that returns the recommended subcommands.
3. Do not execute repairs or publish in this slice.
4. Run: `go test ./cmd/agent-testbench -run TestMapMaintain -count=1`.
5. Commit only if the alias feels clearly useful after Tasks 10-12.

**Acceptance:**
- Lifecycle aliases remain optional; metadata and grouped help are the primary product path.

## Final Verification

Run before opening the PR:

```bash
go test ./cmd/agent-testbench ./internal/domain/plangraph ./internal/domain/mapplanner -count=1
go test ./...
make lint
npm run guard:duplicates -- cmd/agent-testbench
./bin/agent-testbench.sh setup --repo /Users/zlh/codes/agent-testbench --build-runtime --runtime-only
./.runtime/bin/agent-testbench status --json
./.runtime/bin/agent-testbench map --help
./.runtime/bin/agent-testbench task suggest --goal "maintain map" --json
```

## Recommended PR Order

1. PR 1: Tasks 10-12, because map lifecycle grouping gives immediate value and low behavior risk.
2. PR 2: Tasks 5-7, because task catalog/suggest/plan are non-executing.
3. PR 3: Tasks 1-4, because shrinking the default surface should happen after replacement paths are visible.
4. PR 4: Tasks 8-9, because task run execution needs more careful safety review.

## Stop Conditions

- Any existing command becomes uncallable.
- `commands --all --json` loses a previously supported command without an explicit compatibility decision.
- A task wrapper executes publish, destructive restore, or remote writes without an explicit `--execute` style opt-in.
- Duplicate-code scan reports new clones in touched CLI files.

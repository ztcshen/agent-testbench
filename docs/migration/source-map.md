# Migration Source Map

This file prevents the project from drifting into a pure greenfield rewrite.
Every non-trivial core capability should map back to the source repository, or
explicitly explain why it is new-core-only.

Source repository:

```text
/Users/zlh/codes/scf-chain-sandbox
```

## Classification

- `ported-and-scrubbed`: behavior was moved from the source repo and domain
  terms were removed or moved behind a profile.
- `reimplemented-with-rationale`: source behavior was used as a contract, but
  code was rewritten because the source was too coupled to domain/runtime state.
- `new-core-only`: the capability did not exist in the source repo and belongs
  in the generic open core.
- `profile-only`: the capability or data should remain in a profile and must
  not move into core.
- `needs-audit`: current implementation exists but must still be checked
  against the source repo before more work builds on top of it.

## Current Components

| Target | Status | Source files to inspect | Notes |
| --- | --- | --- | --- |
| `cmd/otsandbox` | needs-audit | `cmd/sandboxctl/main.go`, `cmd/sandboxctl/*_test.go` | CLI shape was built neutrally, but each command should be checked against source command behavior before expansion. |
| `internal/store` | needs-audit | `internal/store/store.go`, `internal/store/store_test.go` | Store was simplified for the new core. Audit old runtime tables before adding more fields or migrations. |
| `internal/profile` | needs-audit | `cmd/sandboxctl/template_config_export.go`, `internal/store/store.go` template catalog types | Profile loader should follow exported bundle contracts, not invent a parallel schema. |
| `internal/evidence` | needs-audit | `internal/store/store.go`, control-plane evidence APIs, `cmd/sandboxctl` evidence commands | Import currently targets a legacy SQLite shape; verify against real source runtime DB and evidence links. |
| `internal/apicase` | needs-audit | `internal/apicase`, `cmd/sandboxctl/main.go` run-case path | Runner is intentionally small. Before expanding assertions or fixtures, port source behavior or document why not. |
| `internal/requesttemplate` | needs-audit | template-config request template records and case authoring API | Rendering semantics must match source request-template behavior where generic. |
| `internal/controlplane` | needs-audit | `internal/controlplane`, `control-plane/static` | Current Control plane is a placeholder shell. Migrate shared templates only after scrub and density checks. |
| `profiles/scf-chain` | profile-only | `control-plane/template-config`, exported profile bundle | This is allowed to contain source-domain terms because it is profile data. |

## Required Before Next Core Expansion

1. Pick one `needs-audit` component.
2. Read the listed source files.
3. Add a focused source mapping note:
   - source behavior kept;
   - source behavior dropped;
   - source behavior deferred;
   - compatibility test or CLI proof.
4. Only then implement the next code slice.

# Quick Start

This guide starts from an empty checkout and runs a neutral local profile. It
does not require a hosted service or a team-owned profile bundle.

## Prerequisites

- Go matching `go.mod`
- Node.js 20 or newer
- npm

Install JavaScript dependencies once:

```sh
npm ci
```

## Verify the Checkout

```sh
./bin/otsandbox.sh version
npm run release-check
```

The release check runs Go tests, the source-domain guardrail, the React build,
and a headless browser smoke test against a generated generic profile.

## Create a Local Store

```sh
tmpdir=$(mktemp -d)
store="$tmpdir/store.sqlite"
./bin/otsandbox.sh store status --store-url "$store"
./bin/otsandbox.sh store upgrade --store-url "$store"
```

SQLite is the default local Store. It is runtime state and should not be
committed.

## Create and Install a Profile

```sh
profile_dir="$tmpdir/profile"
./bin/otsandbox.sh profile init \
  --output "$profile_dir" \
  --id sample \
  --display-name "Sample Profile"

./bin/otsandbox.sh profile install --from "$profile_dir"
./bin/otsandbox.sh profile verify --profile sample --store-url "$store"
```

The core repository intentionally ships without bundled profiles. A profile is
the source-owned configuration bundle for services, workflows, interface nodes,
API cases, templates, fixtures, and bindings.

## Start the Workbench

```sh
./bin/otsandbox.sh serve \
  --profile sample \
  --store-url "$store" \
  --host 127.0.0.1 \
  --port 18191
```

Open `http://127.0.0.1:18191/`.

## Next Steps

- Read [profile-authoring.md](profile-authoring.md) to build a real bundle.
- Read [cli-api-contracts.md](cli-api-contracts.md) before wiring an agent or
  CI job to the sandbox.
- Read [api-case-format.md](api-case-format.md) for runnable case files and
  Evidence output.

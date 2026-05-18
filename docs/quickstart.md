# Quick Start

This guide starts from an empty checkout and runs a neutral local import bundle. It
does not require a hosted service or a team-owned import bundle bundle.

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
npm run demo:api-case
npm run release-check
```

The release check runs Go tests, the source-domain guardrail, the React build,
and a headless browser smoke test against a generated generic import bundle.
The demo command starts a temporary local HTTP endpoint, runs the generic
`examples/api-cases/create-item.json` case, and prints the Evidence bundle path.
Demo output is kept under the system temp directory so you can inspect it after
the command exits. Set `OTSANDBOX_CLEAN_DEMO_OUTPUT=1` to remove it
automatically.

## Create a Local Store

```sh
tmpdir=$(mktemp -d)
store="$tmpdir/store.sqlite"
./bin/otsandbox.sh store status --store-url "$store"
./bin/otsandbox.sh store upgrade --store-url "$store"
```

SQLite is the default local Store. It is runtime state and should not be
committed.

## Create and Install a Import Bundle

```sh
import bundle_dir="$tmpdir/import bundle"
./bin/otsandbox.sh import bundle init \
  --output "$import bundle_dir" \
  --id sample \
  --display-name "Sample Import Bundle"

./bin/otsandbox.sh import bundle install --from "$import bundle_dir"
./bin/otsandbox.sh import bundle verify --import bundle sample --store-url "$store"
```

The core repository intentionally ships without bundled import bundles. A import bundle is
the source-owned configuration bundle for services, workflows, interface nodes,
API cases, templates, fixtures, and bindings.

## Start the Workbench

```sh
./bin/otsandbox.sh serve \
  --import bundle sample \
  --store-url "$store" \
  --host 127.0.0.1 \
  --port 18191
```

Open `http://127.0.0.1:18191/`.

## Next Steps

- Read [import bundle-authoring.md](import bundle-authoring.md) to build a real bundle.
- Read [cli-api-contracts.md](cli-api-contracts.md) before wiring an agent or
  CI job to the sandbox.
- Read [api-case-format.md](api-case-format.md) for runnable case files and
  Evidence output.

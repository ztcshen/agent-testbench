# AgentTestBench Desktop

The frontend is an Electron application with a new React workspace. It uses a
project navigation rail, service/interface tree, multiple editor tabs, request
parameter tables, a JSON request editor, response panes, scenario steps and run
reports. Its layout follows the [Apifox page layout](https://docs.apifox.com/page-layout)
and [scenario editor](https://docs.apifox.com/new-test-scenario). The implementation
is original and does not embed the commercial application.

From a source checkout with Node.js and Go installed:

```sh
npm ci
npm run desktop:start
```

`npm run desktop:build` builds the renderer and the Go execution engine.
`npm run desktop:package` produces the native application under
`dist/desktop-release/`. A packaged application includes its execution engine and
does not need Go, Node.js or a separately running browser server. `npm run
build:frontend` now builds only the Electron renderer. Older browser entrypoints
return HTTP 410; the control-plane root identifies itself as an Electron API
workspace. Old source files are outside the active frontend build.

The application normally owns one loopback Go child process. Its default SQLite
Store and Evidence live in its OS application-data directory, independently of
any configured team Store. On an empty Store it registers one generic HTTP
service through the existing API. Closing the application stops only the child
process it started. Project settings can select an existing named Store or DSN,
or connect to an existing API service. Attached services are never stopped by the
desktop application. Unsaved edits and active runs block a workspace switch.

Request edits use `GET` and `PUT /api/request-editor`. PUT requires the catalog's
strong `If-Match` ETag and writes the interface, API case and native execution
configuration as one Store revision. Request bodies, parameters, headers,
expected HTTP codes, response assertions and timeouts are editable. Saved
credentials are masked and retained when the displayed mask is saved back.
Existing external adapters remain runnable through their original Store case,
with their adapter-owned configuration protected from the HTTP editor. No Store
schema change is required.

Each open tab retains its draft. A catalog refresh preserves local edits and
their original ETag if the underlying request has changed. Conflicting saves
return a visible error and do not overwrite the newer version. Requests execute
through the existing Store-native `/api/test-kit/run` API. Target addresses are
saved in the Store; the public execution payload does not accept trusted target
or Evidence-directory overrides.

Scenario editing uses the existing `/api/workflow-studio` contract. Add registered
cases, reorder their steps, select a preceding response/request body field for a
template parameter, and save to the Store. Running uses asynchronous batch APIs.
The new step editor shows execution results separately from environment
acceptance; missing real SkyWalking topology remains an incomplete acceptance.
The report viewer reads existing persisted run and Evidence APIs.

The renderer has no Node.js integration. A sandboxed, isolated preload exposes
specific IPC methods. The main process checks the invoking frame, confines API
requests to the configured backend, rejects navigation/new windows, and uses a
local content security policy. This follows
[Electron's security guidance](https://www.electronjs.org/docs/latest/tutorial/security).
The desktop process structure also references the mature open-source
[Bruno Electron application](https://github.com/usebruno/bruno/tree/main/packages/bruno-electron).

For background interaction checks, set `AGENT_TESTBENCH_DESKTOP_HEADLESS=1` when
launching Electron. Windows remain hidden while Playwright exercises the real
desktop renderer. `npm run test:frontend` tests the request draft/conflict model
and IPC boundaries. Go endpoint tests cover atomic saves, stale revisions, masked
credential preservation and actual HTTP execution.

Run a repeatable desktop integration check with `npm run smoke:desktop`. It
starts a temporary HTTP target and an isolated Store, exercises real Electron
request editing, tab persistence, scenario bindings, reports and environment
registration, and checks that the owned engine stops on exit. To check a packaged
executable, pass `-- --app=PATH_TO_EXECUTABLE --screenshots=OUTPUT_DIRECTORY`.
The fixture performs no requests against configured team targets.

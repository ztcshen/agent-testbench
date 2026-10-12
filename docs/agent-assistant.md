# Store-backed LangChain assistant

The Electron map workspace includes an AI assistant for dependency analysis,
deterministic test-plan previews and diagnosis of persisted failures. It runs the
actual LangChain JavaScript `createAgent` tool loop in an isolated Electron
utility process. The desktop renderer retains its sandbox and has no Node.js
integration. LangChain, its OpenAI-compatible model client and the worker are
bundled with the native application; no Python or separate Node installation is
required.

The model settings also support **Codex · 本机 ChatGPT 登录**. This mode reads the
current file-backed Codex login at invocation time and sends requests only to the
Codex backend. It neither copies nor updates the login cache and does not ask for
a Platform API key. The transport adapts Codex Responses messages and tool calls
to LangChain; private reasoning continuity remains internal to the current model
turn. A system HTTPS proxy is used when available, with an optional explicit
proxy setting. Invalid/expired login produces a visible error.

The Codex transport uses the maintained Pi AI provider and its normalized
transcript contract, preserving both system instructions and registered tools.
Its tests assert these fields reach the provider, preserve tool-call identity
and reject truncated tool responses. `AGENT_TESTBENCH_MODEL_PROVIDER=codex`
selects this transport for development. Model ID remains explicit; no model is
silently replaced.

Open a map and click **AI 图谱助手**, then **模型设置**. Enter the service's Base
URL, model ID and optional API key. The endpoint must support the Chat Completions
tool-call protocol. The connection check sends only a generic greeting; it does
not send map data. A successful connection check alone does not establish that a
model supports the complete tool/structured-output workflow.

The three shortcuts analyze dependencies, build a plan, and diagnose failures.
The assistant invokes these registered tools:

| Tool | Existing capability |
| --- | --- |
| `read_map` | Safe selected-map/profile projection, bounded nodes, paths and case metadata |
| `explain_node_dependencies` | Existing `plangraph.ExplainCase` |
| `build_test_plan` | Existing deterministic `mapplanner.Explain`, without saving or executing the plan |
| `read_recent_runs` | Persisted case runs filtered by the selected map, profile, node/path |
| `read_run_evidence` | Bounded technical assertion and Evidence metadata for a matching stored case run |

All tool calls are bound to the session's Store map and profile. Arbitrary shell,
network, case execution and graph editing tools are absent. Technical evidence
excludes raw request bodies and credentials. A model's output is a typed result;
observations require references returned by successful tools. Unknown node and
Evidence references fail the turn. Hypotheses remain labeled as hypotheses.
Assistant completion never changes an API-case status or environment acceptance.

Plans come from the existing Go Planner. The model selects and explains the
preview; it does not define executable physical tasks. A captured graph
fingerprint prevents an analysis from silently mixing different graph revisions.

## Persistence and lifecycle

Shared SQL Store schema version 23 adds `assistant_sessions`. The same table and
methods support SQLite, PostgreSQL and MySQL. It stores the map/profile binding,
desktop owner ID, messages, tool events, validated analysis, plan previews and a
revision number. Updates compare the expected revision; stale writers receive
409. Conversation and completed results survive application restarts.

This first stage restores conversation/results, rather than resuming a model
invocation at an exact internal graph checkpoint. Interrupted turns are marked
explicitly and can be analyzed again. Only one turn starts per desktop at a time.
Canceling, switching Store or quitting aborts the worker and checkpoints the
terminal status before stopping the owned engine.

The application bounds model timeout, output tokens, graph context, tool calls
and total turn duration. LangSmith tracing is disabled in the worker by default.
The model credential stays in the desktop process/worker. Electron `safeStorage`
encrypts it when available. Otherwise it remains only in process memory and must
be entered again after restart. Public renderer settings contain only a presence
flag. Changing service origin without a new key clears the stored credential.

For local development, optional settings are
`AGENT_TESTBENCH_MODEL_BASE_URL`, `AGENT_TESTBENCH_MODEL_ID` and
`AGENT_TESTBENCH_MODEL_API_KEY`. These override the model connection at runtime;
the key is not written to the Store or the preferences file.

## API contracts

| Method | Route | Contract |
| --- | --- | --- |
| GET / POST | `/api/assistant/sessions` | List this owner/map's session summaries, or create an idle conversation bound to an existing map |
| GET / PUT | `/api/assistant/session?id=...` | Read a conversation or replace state using its expected `revision` |
| POST | `/api/assistant/tool` | Invoke one registered read-only action, bound by `sessionId`; cross-profile case runs and unknown node/path IDs are rejected |

The assistant uses the existing desktop IPC frame checks. API storage routes do
not accept model credentials. Models are configured through the desktop API.

## Validation

`npm run test:frontend` includes the actual LangChain model/tool protocol tests,
reference validation, cancellation and credential-storage tests. Go endpoint
tests cover session persistence, stale updates, redaction, profile boundaries and
graph-version changes.

`npm run smoke:assistant` runs the real Electron utility process against an
isolated SQL Store and a synthetic OpenAI-compatible service. It verifies tool
calls, Go Planner output, no case execution, restart recovery, model settings,
concurrent starts and cancellation. Pass `-- --app=EXECUTABLE --report=JSON_PATH`
to test a packaged application. This protocol fixture is not a live LLM quality
evaluation and never contacts a configured team target or model service.

The design follows the mature [LangChain agent implementation](https://github.com/langchain-ai/langchainjs/tree/main/libs/langchain/src/agents),
[structured-output contract](https://docs.langchain.com/oss/javascript/langchain/structured-output)
and [Electron utility-process boundary](https://www.electronjs.org/docs/latest/api/utility-process).

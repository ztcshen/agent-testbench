# AgentTestBench Feedback

Durable feedback registered by local Codex sessions. Use
`skills/agent-testbench-feedback/scripts/register_feedback.py` for new entries.

## 2026-05-28 - Environment component graph and compose plan can diverge
- Area: environment restore
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: `environment components inspect` showed required dependency components and app nodes, but the recorded compose execution plan only started the app compose files. Restore generated dependency assets and later failed because a required dependency service was not running.
- Suggestion: Add a restore readiness item that compares required component `composeService` values with the compose service allow-list and compose files, then prints a concrete repair hint before Docker starts.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreRejectsRequiredComposeServiceGaps`

## 2026-05-28 - Sandbox start and environment component graph use different registries
- Area: sandbox cli
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: `sandbox start --service <dependency>` failed even though the environment component graph contained that dependency; other dependency entries could also be skipped because their profile service startup command was empty.
- Suggestion: The missing-service error now explains the registry boundary, and `sandbox service list --environment ENV_ID --include-components` gives a read-only bridge view that shows profile services beside environment component-graph-only dependencies.
- Verification: `go test ./cmd/agent-testbench -run 'TestSandbox(ServiceListCanIncludeEnvironmentComponentGraph|ServiceListReportsRegisteredServicesReadOnly|StartMissingServiceExplainsRegistryBoundary)' -count=1`

## 2026-05-28 - Environment restore health wait needs progress output
- Area: environment restore
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: A target service health URL could already return `200 UP` while restore kept waiting without showing the active probe, latest HTTP status or error, or remaining timeout.
- Suggestion: Emit health-check progress to stderr for non-JSON `environment restore --execute` runs, including the target, latest status/error, and completion state.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreHealthWaitReportsProgress`

## 2026-05-28 - Case run should fail fast for bodyless write requests
- Area: case run
- Severity: P1
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: `case run --case-id ...` produced `body=null` for a POST case and the target returned HTTP 400 even though the case appeared ready.
- Suggestion: After request-template rendering and patching, fail before sending HTTP when POST, PUT, or PATCH has no rendered body, and tell the user to add `caseExecution.body` or a body-rendering request template.
- Verification: `go test ./internal/server/controlplane -run TestServerTestKitRunFailsFastForBodylessWriteRequest`

## 2026-05-28 - hasExecutionConfig should not mean bodyless write cases are runnable
- Area: case suite
- Severity: P1
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: Case suite inspection could report `hasExecutionConfig=true` when the config only contained method/path metadata and no POST body.
- Suggestion: Treat active execution configs as runnable only when they have execution metadata and, for POST/PUT/PATCH, a non-null `caseExecution.body`.
- Verification: `go test ./internal/domain/casesuite -run TestExecutionConfigSetDoesNotMarkBodylessWriteConfigRunnable`

## 2026-05-28 - Local evidence URI lifecycle is unclear
- Area: evidence
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: `case evidence` listed historical passed-run request/response attachment URIs, but the local `/tmp/.../request.json` and `response.json` files had been deleted; `case diagnose` could not read them.
- Suggestion: Mark local file evidence lifecycle in Store metadata and add a command or diagnostic next action to export, copy, or rebuild evidence before temporary files disappear.
- Verification: `go test ./internal/server/controlplane -run TestServerMarksMissingLocalEvidenceLifecycle -count=1`; `go test ./cmd/agent-testbench -run TestCaseDiagnoseReportsExpiredLocalEvidenceNextAction -count=1`

## 2026-05-28 - HTTP 200 alone can hide business failure
- Area: case assertions
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-28
- Evidence: A case passed by HTTP status while downstream data showed a FAILED business decision because a dependent response lacked an expected decision field.
- Suggestion: Add Store-backed post-run assertions such as SQL checks against application-visible state, so case suite reports can require both transport success and business-state success.
- Verification: `docs/api-case-format.md`; existing gate coverage `go test ./internal/server/controlplane -run TestServerTestKitRunHonorsExpectedResponseContains -count=1`

## 2026-05-29 - Environment restore JSON adoption should fail with bounded health evidence
- Area: environment restore
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-29
- Evidence: A JSON `environment restore --execute --use-existing-containers` run could wait longer than the requested health timeout while a command-style health probe was still running, leaving stdout empty until the process was killed.
- Suggestion: Bound the whole restore health phase, cap command probes with the remaining timeout, and surface the failing health target in the final JSON report plus `summary.lastRestore`.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreCommandHealthTimeoutBoundsSlowProbe`

## 2026-05-29 - Runtime SQL discovery and run-scoped Evidence checks should be in the runbook
- Area: docs
- Severity: P3
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-29
- Evidence: `runtime mysql endpoints --include-tables --json` and `evidence list --run RUN_ID --json` gave enough Store-backed diagnostics to verify runtime database visibility and request/response Evidence before inspecting Docker or local files.
- Suggestion: Document those commands as preferred first checks for sandbox diagnostics.
- Verification: `docs/quickstart.md`

## 2026-05-29 - Sandbox service registration needs a read-only list and startup dry-run
- Area: sandbox cli
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-29
- Evidence: After registering services, operators had no obvious read-only service catalog command and `sandbox start` could execute unrelated startup commands while checking registration state.
- Suggestion: Add `sandbox service list`/`discover --json` and `sandbox start --dry-run` so registration state and startup plans can be inspected without launching services.
- Verification: `go test ./cmd/agent-testbench -run 'TestSandbox(ServiceListReportsRegisteredServicesReadOnly|StartDryRunDoesNotRunStartupCommands)'`

## 2026-05-29 - Workflow creation needs small upsert commands
- Area: workflow cli
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-29
- Evidence: Adding one smoke workflow still requires exporting the full profile catalog, editing `profile.json`, and importing the whole profile with audit.
- Suggestion: Add Store-first `workflow register/upsert` and workflow binding register/upsert commands with `--json` and `--audit` support so small workflow additions do not require whole-profile import.
- Verification: `go test ./cmd/agent-testbench -run 'TestWorkflow(RegisterAndBindingUpsertStoreCatalog|BindingAuditReportsMissingReferences)' -count=1`

## 2026-05-29 - Component MySQL assets need graceful incremental ALTER workflow
- Area: environment migration
- Severity: P2
- Status: fixed
- Source: local AgentTestBench usability note from 2026-05-29
- Evidence: Component-to-MySQL edges could execute SQL assets during restore, but adding one ALTER required whole-graph editing and manual idempotency; there was no small versioned migration command, target history table, checksum guard, or baseline path.
- Suggestion: Add Store-first versioned MySQL migration assets linked from component dependency edges, with add/list/plan/apply/baseline CLI commands and restore integration.
- Verification: `go test ./cmd/agent-testbench -run 'TestEnvironment(Migration|RestoreAppliesMySQLMigration)'`

## 2026-05-30 - Hermes-style CLI P0 operator entrypoints
- Area: cli
- Severity: P1
- Status: fixed
- Source: User requested Hermes CLI transformation checklist on 2026-05-30
- Evidence: The CLI needed top-level status, doctor, update release mode, searchable commands, and copyable examples so new operators can start without reading the full help output.
- Suggestion: P0 is fixed by status and doctor commands, update --release latest, help Examples, command catalog example filtering, README/quickstart/CLI docs, and tests under cmd/agent-testbench.
- Verification: `go test ./cmd/agent-testbench -run 'Test(StatusReportsRepoRuntimeAndStoreSummary|DoctorReportsMissingActiveStoreWithoutFailing|UpdateReleaseLatestResolvesHighestRemoteTag|TopLevelHelpShowsStoreFlagNotLegacyStoreURL|CommandsCommandEmitsSearchableCommandCatalog)' -count=1`; `go test ./... -count=1`; `make quality`

## 2026-05-30 - Hermes-style CLI P1 setup and repair workflow
- Area: cli
- Severity: P2
- Status: fixed
- Source: User requested Hermes CLI transformation checklist on 2026-05-30
- Evidence: After P0, the next usability gap is helping a clean machine self-repair common setup issues without manual doc spelunking.
- Suggestion: P1 is fixed by `setup`, `doctor --fix`, `update --channel main|release`, clearer `update --check` next actions, dirty-check repair guidance, `commands --area`, and quickstart/contract docs for clean-machine operation.
- Verification: `go test ./cmd/agent-testbench -run 'Test(CommandsCanFilterByArea|UpdateChannel|UpdateCheckText|UpdateRejectsTrackedLocalChangesWithoutForce|DoctorFix|StatusDeep|SetupConfigures)' -count=1`; `go test ./... -count=1`; `make quality`

## 2026-05-30 - Hermes-style CLI P2 operational depth
- Area: cli
- Severity: P3
- Status: fixed
- Source: User requested Hermes CLI transformation checklist on 2026-05-30
- Evidence: Longer-term operator ergonomics need deeper diagnostics and shell integration once the basic entrypoints are stable.
- Suggestion: P2 is fixed for the Hermes CLI baseline by shell completion, `logs`, `doctor --deep`, stable doctor check codes, `config path/show/edit`, `status --deep`, and documentation that points operators at existing run/case/workflow/evidence query commands instead of chat-session instructions.
- Verification: `go test ./cmd/agent-testbench -run 'Test(CompletionPrints|Logs|Config|MainHelpIncludesP2|DoctorFix|StatusDeep)' -count=1`; `go test ./... -count=1`; `make quality`

## 2026-05-29 - MySQL migration apply leaves plan pending
- Area: store
- Severity: P2
- Status: fixed
- Source: private validation table-prefix migration via environment migration apply
- Evidence: `environment migration apply --execute` returned applied, but the next `environment migration plan` still reported the same migration as pending because the Store asset status was not updated after target history changed.
- Suggestion: Successful migration apply/baseline now persists completed asset status back to the Store, and `plan` filters applied or baselined versions.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentMigrationApplyPersistsStatusForPlan -count=1`

## 2026-05-29 - Sandbox service start skips services without editable startup command
- Area: environment
- Severity: P2
- Status: fixed
- Source: private validation workflow smoke run, 2026-05-29
- Evidence: `sandbox service list` could show active services with `hasStartupCommand=false`, and `sandbox start --dry-run` skipped them.
- Suggestion: `sandbox service register --startup-command ...` is an idempotent repair path for empty startup metadata; the regression test protects that a repaired service becomes planned by `sandbox start --dry-run`.
- Verification: `go test ./cmd/agent-testbench -run TestSandboxServiceRegisterCanRepairStartupCommand -count=1`

## 2026-06-01 - Environment restore generated Kafka image tag is unavailable
- Area: environment
- Severity: P2
- Status: fixed
- Source: private validation smoke restore via agent-testbench-operator on 2026-06-01
- Evidence: Docker Compose pull failed late with an unavailable Kafka image tag.
- Suggestion: `environment restore --execute` now validates image manifests for services that would be pulled and fails before `docker compose pull` with a concrete image/service message.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreReportsUnavailableComposeImageBeforePull -count=1`

## 2026-06-01 - Environment restore emits host-specific bind mounts despite repo env overrides
- Area: environment
- Severity: P2
- Status: fixed
- Source: private validation smoke restore via agent-testbench-operator on 2026-06-01
- Evidence: Generated Compose files could retain a previous operator machine's absolute repo path even when the current environment recorded repo checkout facts.
- Suggestion: Restore now rewrites generated Compose bind sources to the current registered component checkout when the source path identifies that service, and rejects any remaining missing absolute host bind sources before Docker starts.
- Verification: `go test ./cmd/agent-testbench -run 'TestEnvironmentRestore(RewritesGeneratedHostBindMountsToRegisteredCheckouts|RejectsMissingHostBindMountBeforeComposeUp)' -count=1`

## 2026-06-01 - Hermes update entrypoint is not reliable across wrappers
- Area: cli
- Severity: P2
- Status: fixed
- Source: 2026-06-01 local operator check after user asked why Hermes-style update does not auto-update
- Evidence: Repo wrapper ./bin/agent-testbench.sh exposes update, but /Users/zlh/.codex/skills/agent-testbench-operator/scripts/atb.sh prefers a stale skill binary where update is unknown; status also reports .runtime/bin/agent-testbench missing, and release channel defaults to origin where no remote tags are available.
- Suggestion: `doctor` now reports `runtime.shell-entrypoint` when `agent-testbench` is missing from PATH or resolves to a stale wrapper, `status` reports the active executable beside the expected runtime binary, and `update --channel main|release` prefers a configured `github` remote before `origin` when no upstream is set.
- Verification: `go test ./cmd/agent-testbench -run 'Test(DoctorWarnsWhenShellEntrypointIsStale|UpdateDefaultsToGithubRemoteWhenNoUpstream)' -count=1`

## 2026-06-01 - Lightweight sandbox workflow cannot be rebuilt after tmp-backed assets disappear
- Area: environment
- Severity: P2
- Status: fixed
- Source: local AgentTestBench operator feedback after a machine reboot
- Evidence: A registered lightweight workflow still existed in the Store, but the previously running containers depended on temporary workspace bind mounts and generated startup assets that were gone after reboot. `sandbox service list` showed the required app service without a startup command, and the CLI had no workflow-scoped startup path to regenerate only the services required by that workflow.
- Suggestion: `sandbox start --workflow WORKFLOW_ID` now selects only services referenced by the workflow's bound interface nodes, so operators can dry-run or execute the lightweight workflow startup path without launching unrelated services. Workflow-required services with missing startup commands now block with a repair hint instead of looking startable.
- Verification: `go test ./cmd/agent-testbench -run 'TestSandboxStart(SelectedServiceFailsWhenStartupCommandMissing|WorkflowBlocksMissingStartupCommand|DryRunDoesNotRunStartupCommands|CommandRunsStartupCommandsFromStore)' -count=1`; `docs/quickstart.md`; `docs/cli-api-contracts.md`; `skills/agent-testbench-operator/references/operator-runbook.md`

## 2026-06-01 - sandbox start returns ok while required service is skipped due empty startup command
- Area: environment
- Severity: P1
- Status: fixed
- Source: local AgentTestBench operator feedback for workflow-bound sandbox startup
- Evidence: `sandbox service list --service SERVICE_ID` could show an active service with `hasStartupCommand=false`, while `sandbox start --service SERVICE_ID --json` returned `ok=true`, `started=0`, and `skipped=1` with `skipReason=startup command is empty`. That made an integration workflow appear successfully started even though its required app service could not be launched by the CLI.
- Suggestion: Explicit `--service` targets and workflow-required services now fail the command when their startup command is empty, returning `ok=false`, a failed count, and a concrete `sandbox service register --id SERVICE_ID --startup-command ...` repair hint.
- Verification: `go test ./cmd/agent-testbench -run 'TestSandboxStart(SelectedServiceFailsWhenStartupCommandMissing|WorkflowBlocksMissingStartupCommand|DryRunDoesNotRunStartupCommands|CommandRunsStartupCommandsFromStore)' -count=1`

## 2026-06-02 - status/doctor cannot detect stale runtime binary after source update
- Area: cli
- Severity: P2
- Status: fixed
- Source: local post-merge runtime freshness verification
- Evidence: A checkout could be on a newer Git revision while `.runtime/bin/agent-testbench` still contained an older build. `status` reported the active executable path as matching the runtime path, and `doctor` did not warn, so other local sessions could continue using stale command behavior until the runtime was manually rebuilt.
- Suggestion: `status` now reports runtime freshness from the runtime binary modification time versus the current Git HEAD commit time, and `doctor` emits a `runtime.fresh` warning with an `onboard --build-runtime --install-shell` repair hint when the runtime predates HEAD.
- Verification: `go test ./cmd/agent-testbench -run 'TestStatusAndDoctorWarnWhenRuntimeBinaryPredatesHead|TestDoctorWarnsWhenShellEntrypointIsStale|TestStatusReportsRepoRuntimeAndStoreSummary' -count=1`

## 2026-06-02 - Operator retry can stop after workflow failure without durable feedback
- Area: skill
- Severity: P2
- Status: fixed
- Source: local operator workflow retry feedback
- Evidence: An operator session could summarize a repeated workflow failure and updated report paths, but stop without matching or registering durable feedback for the unrecoverable blocker.
- Suggestion: The operator skill and runbook now require matching or registering feedback before stopping on repeated or unrecoverable CLI-diagnosed blockers, including the blocking command, workflow run id, failed step/case id, service id, Store state, exact error, and next repair command.
- Verification: `skills/agent-testbench-operator/SKILL.md`; `skills/agent-testbench-operator/references/operator-runbook.md`

## 2026-06-02 - environment restore should accept locally available Docker images when registry is unreachable
- Area: environment
- Severity: P1
- Status: fixed
- Source: local Docker-backed smoke restore feedback
- Evidence: `environment restore --execute` could block Docker startup when registry manifest checks timed out even though `docker image inspect` showed all required images already existed locally.
- Suggestion: Docker image preflight now falls back to `docker image inspect IMAGE` when registry manifest probing fails, so an already-present local image is accepted with a structured note instead of blocking compose startup.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreAcceptsLocalComposeImageWhenRegistryProbeFails -count=1`

## 2026-06-02 - environment restore clean-docker-state misses fixed-name containers from another compose project
- Area: environment
- Severity: P2
- Status: fixed
- Source: local Docker-backed smoke restore feedback
- Evidence: `--clean-docker-state --allow-destructive-docker-cleanup` only planned `docker compose down` for the current project, so a fixed `container_name` already held by another compose project could still break the subsequent `docker compose up`.
- Suggestion: `--clean-docker-state` now includes detected fixed `container_name` conflicts in the cleanup report and appends an explicit `docker rm -f NAME...` cleanup command when destructive cleanup is allowed.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreCleanupRemovesConflictingFixedContainerNames -count=1`

## 2026-06-02 - workflow runner needs first-class MQ and Kafka trigger steps
- Area: workflow
- Severity: P1
- Status: fixed
- Source: local message-driven workflow smoke feedback
- Evidence: Workflow register/upsert can create a dedicated workflow, but workflow bindings are still API-case oriented. Non-HTTP message triggers must currently live outside workflow report execution.
- Suggestion: `workflow task run --workflow ID --step STEP=TASK_NAME_OR_ID` now runs Store-backed task steps in workflow order, records a workflow run summary with task run ids/status, stores per-step task Evidence under the workflow run, and lets `workflow gate --require-evidence` count task-step Evidence. MQ/Kafka trigger commands remain generic shell/CLI tasks rather than protocol-specific hardcoding.
- Verification: `go test ./cmd/agent-testbench -run TestWorkflowTaskRunRecordsShellTriggerAndPostconditionSteps -count=1`; `go test ./cmd/agent-testbench -run TestCommandsCommandEmitsSearchableCommandCatalog -count=1`

## 2026-06-02 - task run cannot cover non-HTTP sandbox trigger commands for MQ smoke
- Area: workflow
- Severity: P1
- Status: fixed
- Source: local message-driven workflow smoke feedback
- Evidence: Store-backed `task run` interpreted a shell-style message publish command as an `agent-testbench` subcommand, so operators could not trigger non-HTTP workflow smoke checks through CLI-only task history.
- Suggestion: `task run --shell --command "..."` now stores the task as kind `shell`, executes the command through `/bin/sh -c`, captures output/exit code in Store task history, and keeps the default `cli` mode unchanged.
- Verification: `go test ./cmd/agent-testbench -run TestTaskRunShellExecutesSandboxTriggerCommand -count=1`

## 2026-06-02 - environment restore reports failure after writing generated compose files and passing health checks
- Area: environment
- Severity: P1
- Status: fixed
- Source: local Docker-backed smoke restore feedback
- Evidence: Restore wrote Store-generated compose files into the workspace and passed health checks, but final readiness still reported `store-startup-files` missing because readiness only compared the original Store `generatedFiles` map.
- Suggestion: Restore readiness now treats written workspace compose files and successful generated-file reports as startup-file readiness, and `docker-start-plan` readiness is based on the planned/written compose artifacts instead of later health status.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreStoreStartupFilesAcceptWrittenWorkspaceCompose -count=1`

## 2026-06-02 - workflow gate require-evidence misses case evidence generated by workflow report
- Area: evidence
- Severity: P2
- Status: fixed
- Source: local workflow gate feedback
- Evidence: `workflow report` produced a passed step and indexed case evidence, but `workflow gate --require-evidence` only listed evidence under the workflow run id and therefore reported zero evidence for the step.
- Suggestion: `workflow gate --require-evidence` now loads evidence indexed under the workflow run plus evidence indexed under each workflow case-run id, deduplicates records, and counts case-run scoped Evidence for step completeness.
- Verification: `go test ./cmd/agent-testbench -run TestWorkflowGateCountsEvidenceStoredUnderCaseRunID -count=1`

## 2026-06-02 - Workflow report can pass before async MQ consumer completes
- Area: workflow
- Severity: P1
- Status: fixed
- Source: local message-driven workflow smoke feedback
- Evidence: A workflow report could pass immediately after a publish/trigger request while the asynchronous consumer later failed on a downstream fixture dependency.
- Suggestion: Message workflows can now model the publish trigger and downstream consumer postcondition as ordered workflow task steps. The workflow run only passes when each task step passes, and `workflow gate --require-evidence` fails unless the trigger/postcondition steps wrote Evidence, so a publish response alone no longer has to stand in for end-to-end success.
- Verification: `go test ./cmd/agent-testbench -run TestWorkflowTaskRunRecordsShellTriggerAndPostconditionSteps -count=1`

## 2026-06-02 - Need first-class object storage fixture support for sandbox workflows
- Area: environment
- Severity: P2
- Status: fixed
- Source: local message-driven workflow smoke feedback
- Evidence: A message-driven workflow could reach the application consumer but fail when it attempted to download an expected object-storage fixture from an unavailable external endpoint.
- Suggestion: Restore now recognizes object-storage dependency assets by capability/kind, reads bucket/key metadata from the Store asset, and seeds the object content through the provider component's generic `objectStorage.seedCommand` metadata. The applied asset report records `plan-seed-object-storage` / `seed-object-storage`, target path, bytes, command, attempts, and errors without hardcoding a concrete storage vendor.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreSeedsObjectStorageEdgeAsset -count=1`

## 2026-06-02 - Docker restore can hang on completed one-shot seed service when docker compose ps returns empty
- Area: environment
- Severity: P2
- Status: fixed
- Source: local Docker-backed object fixture restore feedback
- Evidence: A one-shot seed service could complete successfully and disappear from default `docker compose ps --format json SERVICE` output, causing restore health polling to wait until timeout even though the seed container exited 0.
- Suggestion: Compose-service health checks now use `docker compose ps -a --format json SERVICE`; restore also infers one-shot services from Compose `depends_on` entries with `condition: service_completed_successfully`, then treats `State=exited` with `ExitCode=0` as successful only for those completed services or explicitly marked one-shot checks.
- Verification: `go test ./cmd/agent-testbench -run 'TestEnvironmentRestore(AcceptsComposeDependencyCompletedOneShotService|AcceptsExplicitCompletedOneShotComposeServiceHealth|EffectiveHealthChecksUseStartedComposeServices|EffectiveHealthChecksCoverBusinessURLService)' -count=1`; `go test ./cmd/agent-testbench -run TestEnvironmentRestoreRunsMixedHealthProbes -count=1`
- Regression evidence 2026-06-03: a private validation `environment restore ... --execute --pull --clean-docker-state --allow-destructive-docker-cleanup` run returned code 2 after all core services were healthy because an object seed compose-service check stayed `ok=false` with `state=exited`; a follow-up task showed `docker inspect` status `exited 0` and seed logs ended with an object-storage seed success line.
- Resolution 2026-06-03: Added a restore-level regression test using the actual `docker compose ps -a --format json` shape from the run and verified the health evaluator accepts completed one-shot services during `environment restore`.

## 2026-06-02 - Workflow retest blocked by unconfigured required service startup metadata
- Area: environment
- Severity: P2
- Status: fixed
- Source: operator workflow retest run.20260602T110642.520569000Z
- Evidence: a private validation workflow report failed at a message-triggered step because the local test endpoint connection was refused; related port probes were refused; docker ps could not connect to the Docker daemon; `sandbox start --workflow ... --dry-run` correctly returned `ok=false` because a required service had empty startup command metadata.
- Suggestion: `sandbox service register --from-environment ENV_ID --id SERVICE_ID` now copies missing startup metadata from the matching environment component graph, using component `startupCommand` / `startCommand` metadata while preserving explicit CLI values. This gives workflow-required services a Store-first repair path before rerunning `sandbox start --workflow`.
- Verification: `go test ./cmd/agent-testbench -run TestSandboxServiceRegisterRepairsStartupCommandFromEnvironmentComponent -count=1`
## 2026-06-04 - Sandbox pass can miss local runtime failure
- Area: environment
- Severity: P1
- Status: fixed
- Source: User report during private validation plus operator status baseline on local Store.
- Evidence: User reported latest code passed sandbox validation but the developer local environment could not run afterward. Operator baseline: store current returned local/sqlite, docker info succeeded, status reported runtime.fresh=true but activeMatchesRuntime=false and activePath pointed at a Go build cache binary instead of .runtime/bin/agent-testbench. This makes sandbox evidence insufficient to prove the same local runtime/entrypoint developers use.
- Suggestion: `sandbox start --json` and stream reports now include runtime consistency evidence from `status`, including active path, expected runtime path, activeMatchesRuntime, freshness, build/source revision, and repair hints. The operator skill now treats a sandbox pass with mismatched or stale runtime evidence as incomplete validation until the repo runtime is rebuilt or the wrapper points at it.
- Verification: `go test ./cmd/agent-testbench -run TestSandboxStartJSONIncludesRuntimeConsistencyEvidence -count=1`
## 2026-06-05 - Message workflow cannot be rerun when bridge and SQL containers are stopped
- Area: environment
- Severity: P2
- Status: fixed
- Source: private message smoke via agent-testbench-operator on 2026-06-05
- Evidence: A team Store contained a message-driven private workflow. The workflow report failed at a publish step because the local bridge endpoint refused the connection. `sandbox start --workflow ... --dry-run` failed because the bridge service had empty startup metadata. `environment restore --use-existing-containers ...` then failed because a required SQL container was stopped, so the CLI could not recover the already-written workflow without destructive Docker cleanup or manual container startup.
- Suggestion: Environment-bound workflows are now blocked from direct `sandbox start --workflow` and directed to `environment restore ENV_ID --store STORE_NAME_OR_DSN --workspace WORKSPACE --execute --run-workflow --server-url URL`, so the workflow rerun is owned by the environment startup, health, asset, Evidence, and verification gate. For non-environment workflow services, `sandbox service register --from-environment ENV_ID --id SERVICE_ID` remains the Store-first repair path for missing startup metadata.
- Verification: `go test ./cmd/agent-testbench -run 'TestSandbox(Start|Service|Register)' -count=1`; `go test ./cmd/agent-testbench -run TestSandboxStartRejectsEnvironmentBoundWorkflow -count=1`
## 2026-06-05 - environment restore reports object storage metadata failure after s3-seed and workflow succeed
- Area: environment
- Severity: P2
- Status: fixed
- Source: private message restore on 2026-06-05
- Evidence: Docker restore started all private message services and the object-storage seed service exited 0, but environment restore still returned ok=false with error `object storage asset requires provider objectStorage.seedCommand metadata`. A follow-up private workflow run passed 1/1, and database/log evidence showed the consumer, workflow, model decision, and final decision all succeeded.
- Suggestion: Object-storage edge assets without provider `objectStorage.seedCommand` are now accepted when the provider compose service is a `service_completed_successfully` one-shot dependency from the generated Compose graph. The applied asset records `action=object-storage-seed-satisfied-by-compose` and `status=compose-service-completed` instead of failing after Docker startup succeeds.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreAcceptsComposeManagedObjectStorageSeed -count=1`
## 2026-06-05 - Existing-container restore reapplies non-idempotent MySQL SQL assets
- Area: environment
- Severity: P2
- Status: fixed
- Source: local reproduction while reviewing 2026-06-05 restore feedback
- Evidence: environment restore ENV --execute --use-existing-containers against an already-started Docker sandbox returned ok=false in docker phase because apply-mysql-sql retried schema assets and MySQL reported ERROR 1050 table already exists; object-storage asset checks were also planned later in the same report.
- Suggestion: `--use-existing-containers` now skips plain MySQL SQL bootstrap assets instead of reapplying them to an already-started database. The applied asset records `action=skip-mysql-sql-use-existing-containers`, `status=skipped`, and a targeted hint to convert the SQL to an environment migration asset or rerun restore with clean Docker state when the SQL must be applied.
- Verification: `go test ./cmd/agent-testbench -run TestEnvironmentRestoreUseExistingContainersSkipsPlainMySQLSQLAssets -count=1`
## 2026-06-10 - Environment-bound workflow can bypass environment restore gate
- Area: environment
- Severity: P1
- Status: fixed
- Source: private typed-facade cleanup validation on a team Store, 2026-06-10
- Evidence: An environment-bound private workflow had 11 steps in the Store. Direct `sandbox start --workflow` runs reported passed for the registered application services, but service health stayed `000`; several application services exited or failed to reach required upstream dependencies, and some required dependency services had no startup metadata or Store registration. `environment discover` returned no environments. A prior workflow batch completed 11/11 with 0 passed because upstream services were unavailable.
- Suggestion: `sandbox start --workflow` now rejects workflows that are bound to an Environment Catalog entry and tells operators to use `environment restore ENV_ID --store STORE_NAME_OR_DSN --workspace WORKSPACE --execute --run-workflow --server-url URL`, keeping environment startup, health checks, workflow execution, Evidence, and verification on the environment lifecycle gate.
- Verification: `go test ./cmd/agent-testbench -run 'TestSandbox(Start|Service|Register)' -count=1`; `go test ./cmd/agent-testbench -run TestSandboxStartRejectsEnvironmentBoundWorkflow -count=1`
## 2026-06-12 - Restore stream-json lacks long-running phase events
- Area: environment
- Severity: P2
- Status: fixed
- Source: user screenshot report 2026-06-12
- Evidence: environment restore --output-format stream-json can run for a long time without events, leaving operators unable to tell whether it is blocked in compose, health checks, or migration.
- Suggestion: Emit step_started/heartbeat/progress events for compose preparation, Docker compose execution, health waiting, migration application, and workflow verification so stream-json always shows the active phase.
- Resolution: No further code fix is needed. Current restore and migration stream-json output emits phase start/completion events plus waiting observations for Docker compose execution, Docker health, migration execution, and workflow acceptance.
- Verification: `go test ./cmd/agent-testbench -run 'Test(EnvironmentRestoreStreamJSONEmitsAgentEvents|EnvironmentRestoreStreamJSONEmitsComposeExecuteWaitingObservation|EnvironmentRestoreStreamJSONSkipsWorkflowWhenDockerIsNotReady|EnvironmentRestoreStreamJSONEmitsWorkflowAcceptanceWaiting|EnvironmentMigrationApplyStreamJSONEmitsAgentEvents)' -count=1`
## 2026-06-12 - Environment registration file relationships are hard to reason about
- Area: environment
- Severity: P2
- Status: fixed
- Source: user screenshot report 2026-06-12
- Evidence: environment register mixes composeFiles, generatedFiles, and startupFiles references; operators can end up referencing files that are not materialized as durable Store-backed assets.
- Suggestion: Normalize the registration model so every referenced compose/startup/generated file has an explicit Store-backed asset record, projection kind, ownership, and inspectable restore-readiness status.
- Resolution: No further code fix is needed for the current restore safety goal. Current inspect/bootstrap/restore output exposes `fileProjection`; missing Store-backed compose/env/config/secret/include/extends projections block restore and return a repair plan instead of relying on host-local files.
- Verification: `go test ./cmd/agent-testbench ./internal/domain/environmentfiles -run 'Test(EnvironmentInspectAndBootstrapExposeFileProjectionGaps|EnvironmentRestoreSQLStoreRejectsComposeNativeFileProjectionGaps|ProjectionReport)' -count=1`
## 2026-06-12 - Compose down cleanup can break partially linked environments
- Area: environment
- Severity: P1
- Status: fixed
- Source: user screenshot report 2026-06-12
- Evidence: An environment was cleaned with docker compose down --remove-orphans, but application containers were not connected through a complete environment link, causing startup chains such as env variable injection to break.
- Suggestion: Before destructive compose cleanup, require a complete environment linkage/preflight proof for all app containers, env injection paths, and compose project boundaries; otherwise block down/remove-orphans and provide a repair plan.
- Resolution: No further code fix is needed. Destructive restore cleanup and `environment stop --down --remove-orphans` now require complete Store-to-Compose linkage proof; incomplete linkage blocks Docker down/remove-orphans and returns a Store-backed repair plan.
- Verification: `go test ./cmd/agent-testbench -run 'Test(EnvironmentRestoreBlocksAllowedDockerCleanupWithoutCompleteLinkage|EnvironmentRestoreBlocksDockerCleanupWhenComposeNativeProjectionMissing|EnvironmentStopDownBlocksWithoutCompleteLinkage|EnvironmentStopDownRemoveOrphansRequiresExplicitFlags)' -count=1`

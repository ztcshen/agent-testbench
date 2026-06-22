package main

import "strings"

type commandDescriptor struct {
	Command     string
	Usage       string
	Surface     string
	Replacement string
	Reason      string
}

func commandCatalogDescriptors() []commandDescriptor {
	lines := strings.Split(strings.TrimSpace(commandDescriptorRegistryText), "\n")
	out := make([]commandDescriptor, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 2 {
			continue
		}
		command := strings.TrimSpace(fields[0])
		usage := strings.TrimSpace(fields[1])
		command = strings.TrimSpace(command)
		usage = strings.TrimSpace(usage)
		if command == "" || usage == "" {
			continue
		}
		descriptor := commandDescriptor{Command: command, Usage: usage}
		for _, rawMetadata := range fields[2:] {
			key, value, ok := strings.Cut(strings.TrimSpace(rawMetadata), "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "surface":
				descriptor.Surface = strings.TrimSpace(value)
			case "replacement":
				descriptor.Replacement = strings.TrimSpace(value)
			case "reason":
				descriptor.Reason = strings.TrimSpace(value)
			}
		}
		out = append(out, descriptor)
	}
	return out
}

const commandDescriptorRegistryText = `
version	agent-testbench version
setup	agent-testbench setup [--repo PATH] [--store NAME] [--url DSN | --sqlite PATH] [--build-runtime] [--runtime-only] [--json]
onboard	agent-testbench onboard [--repo PATH] [--store NAME] [--url DSN | --sqlite PATH] [--build-runtime] [--install-shell] [--bin-dir PATH] [--smoke none|commands|store] [--json]
status	agent-testbench status [--deep] [--json]	surface=default	reason=orientation: first commands for status, diagnosis, and command discovery
doctor	agent-testbench doctor [--fix] [--deep] [--trace-graphql-url URL] [--json]	surface=default	reason=orientation: first commands for status, diagnosis, and command discovery
update	agent-testbench update [--repo PATH] [--remote NAME] [--branch NAME] [--release TAG|latest] [--channel main|release] [--check] [--force] [--output PATH] [--json]
commands	agent-testbench commands [--area AREA] [--filter TEXT] [--all] [--internal] [--json]	surface=default	reason=orientation: first commands for status, diagnosis, and command discovery
completion	agent-testbench completion [bash|zsh]
logs	agent-testbench logs [NAME|list] [-n N] [--json]
task catalog	agent-testbench task catalog [--filter TEXT] [--json]	surface=default	reason=task intent: lets agents discover, plan, and run repeatable operator tasks
task suggest	agent-testbench task suggest --goal TEXT [--json]	surface=default	reason=task intent: lets agents discover, plan, and run repeatable operator tasks
task plan	agent-testbench task plan TASK_ID [--map ID] [--environment ENV_ID] [--workspace PATH] [--case-run ID] [--run ID] [--store NAME_OR_DSN] [--json]	surface=default	reason=task intent: lets agents discover, plan, and run repeatable operator tasks
task run	agent-testbench task run NAME --command COMMAND [--store NAME_OR_DSN] [--shell] [--notify-file PATH] [--notify-webhook URL] [--json]	surface=default	reason=task intent: lets agents discover, plan, and run repeatable operator tasks
task run	agent-testbench task run TASK_ID [--map ID] [--environment ENV_ID] [--workspace PATH] [--case-run ID] [--run ID] [--store NAME_OR_DSN] [--dry-run] [--json]	surface=default	reason=task intent: lets agents discover, plan, and run repeatable operator tasks
task schedule	agent-testbench task schedule NAME --command COMMAND (--interval DURATION | --cron EXPR) [--store NAME_OR_DSN] [--notify-file PATH] [--notify-webhook URL] [--json]
task watch	agent-testbench task watch NAME --command COMMAND [--store NAME_OR_DSN] [--interval DURATION] [--limit N] [--until always|success|failure] [--notify-file PATH] [--notify-webhook URL] [--json]
task list	agent-testbench task list [--store NAME_OR_DSN] [--json]
task status	agent-testbench task status NAME [--store NAME_OR_DSN] [--json]
task logs	agent-testbench task logs NAME [--store NAME_OR_DSN] [-n N] [--json]
task stop	agent-testbench task stop NAME [--store NAME_OR_DSN] [--json]
notify test	agent-testbench notify test (--file PATH | --webhook URL) [--message TEXT] [--json]	surface=internal
config path	agent-testbench config path
config show	agent-testbench config show [--json]
config edit	agent-testbench config edit
store config set	agent-testbench store config set NAME --url postgres://...
store config set	agent-testbench store config set NAME --url mysql://...
store config set	agent-testbench store config set NAME --url sqlite://PATH
store config list	agent-testbench store config list [--json]
store use	agent-testbench store use NAME
store current	agent-testbench store current [--json]	surface=default	reason=store: identifies the active SQL Store and its health
store status	agent-testbench store status [--store NAME_OR_DSN] [--json]	surface=default	reason=store: identifies the active SQL Store and its health
store provision	agent-testbench store provision [--store NAME_OR_DSN] [--json]
store upgrade	agent-testbench store upgrade [--store NAME_OR_DSN]
store ddl	agent-testbench store ddl [--backend postgres|mysql] [--store NAME_OR_DSN]
store copy	agent-testbench store copy --from NAME_OR_DSN --to NAME_OR_DSN [--require-environment ENV_ID] [--require-verification-workflow ID] [--require-verified-environment] [--require-min-components N] [--require-min-dependencies N] [--require-min-assets N] [--require-inline-asset-bytes N] [--json]
environment register	agent-testbench environment register --id ID [--store NAME_OR_DSN] [--display-name NAME] [--service ID] [--repo SERVICE=PATH] [--branch SERVICE=BRANCH] [--checkout SERVICE=PATH] [--package-repo URL] [--package-branch BRANCH] [--package-ref REF] [--compose-file PATH]... [--compose-generated-file TARGET=SOURCE_FILE]... [--compose-env KEY=VALUE]... [--start-command TEXT] [--health-url URL] [--health-tcp HOST:PORT] [--health-command CMD] [--health-compose-service SERVICE] [--verification-workflow ID] [--json]
environment discover	agent-testbench environment discover [--store NAME_OR_DSN] [--all] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment inspect	agent-testbench environment inspect ENV_ID [--store NAME_OR_DSN] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment bootstrap	agent-testbench environment bootstrap ENV_ID [--store NAME_OR_DSN] [--json]
environment configure	agent-testbench environment configure ENV_ID --view components|repos|startup-files [--repo SERVICE=URL] [--branch SERVICE=BRANCH] [--repo-ref SERVICE=REF] [--checkout SERVICE=PATH] [--file TARGET=SOURCE_FILE|COMPONENT_GRAPH_JSON] [--store NAME_OR_DSN] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment repo set	agent-testbench environment repo set ENV_ID [--repo SERVICE=URL] [--branch SERVICE=BRANCH] [--repo-ref SERVICE=REF] [--checkout SERVICE=PATH] [--store NAME_OR_DSN] [--json]	replacement=agent-testbench environment configure --view repos ENV_ID
environment startup-file put	agent-testbench environment startup-file put ENV_ID --file TARGET=SOURCE_FILE [--store NAME_OR_DSN] [--json]	replacement=agent-testbench environment configure --view startup-files ENV_ID
environment components inspect	agent-testbench environment components inspect ENV_ID [--store NAME_OR_DSN] [--json]	replacement=agent-testbench environment configure --view components ENV_ID
environment components replace	agent-testbench environment components replace ENV_ID --file COMPONENT_GRAPH_JSON [--store NAME_OR_DSN] [--json]	replacement=agent-testbench environment configure --view components ENV_ID --file COMPONENT_GRAPH_JSON
environment migration add	agent-testbench environment migration add ENV_ID --edge OWNER:PROVIDER --database DB --version VERSION --file SQL_FILE [--description TEXT] [--precondition column-not-exists:TABLE.COLUMN] [--store NAME_OR_DSN] [--json]
environment migration list	agent-testbench environment migration list ENV_ID [--edge OWNER:PROVIDER] [--database DB] [--store NAME_OR_DSN] [--json]
environment migration plan	agent-testbench environment migration plan ENV_ID [--edge OWNER:PROVIDER] [--database DB] [--store NAME_OR_DSN] [--json]
environment migration apply	agent-testbench environment migration apply ENV_ID --edge OWNER:PROVIDER --database DB --workspace PATH [--through-version VERSION] [--execute] [--store NAME_OR_DSN] [--output-format text|json|stream-json] [--json]
environment migration baseline	agent-testbench environment migration baseline ENV_ID --edge OWNER:PROVIDER --database DB --workspace PATH [--through-version VERSION] [--execute] [--store NAME_OR_DSN] [--output-format text|json|stream-json] [--json]
environment restore	agent-testbench environment restore ENV_ID --workspace PATH [--store NAME_OR_DSN] [--execute] [--pull] [--prepare-repos-only] [--assume-clean-docker] [--use-existing-containers] [--clean-docker-state] [--clean-docker-images] [--allow-destructive-docker-cleanup] [--run-workflow --server-url URL] [--base-url URL] [--workflow-output-dir PATH] [--health-timeout-seconds N] [--output-format text|json|stream-json] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment status	agent-testbench environment status ENV_ID --workspace PATH [--store NAME_OR_DSN] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment stop	agent-testbench environment stop ENV_ID --workspace PATH [--store NAME_OR_DSN] [--down] [--remove-orphans] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment service restart	agent-testbench environment service restart ENV_ID --workspace PATH --service SERVICE_OR_COMPONENT [--store NAME_OR_DSN] [--health-timeout-seconds N] [--json]	surface=default	reason=environment lifecycle: inspect, restore, check, stop, or restart a registered environment
environment acceptance start	agent-testbench environment acceptance start ENV_ID --server-url URL --request-id ID [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
environment acceptance report	agent-testbench environment acceptance report ENV_ID --server-url URL --run ID [--json]
environment verify	agent-testbench environment verify ENV_ID --run ID --status STATUS [--evidence-complete] [--topology-complete] [--store NAME_OR_DSN] [--json]
environment publish-verified	agent-testbench environment publish-verified ENV_ID [--store NAME_OR_DSN] [--json]
runtime mysql endpoints	agent-testbench runtime mysql endpoints [--include-tables] [--json]	surface=internal	replacement=agent-testbench store status --json
sandbox start	agent-testbench sandbox start [--store NAME_OR_DSN] [--service ID | --workflow ID] [--kind KIND] [--timeout-seconds N] [--dry-run] [--output-format text|json|stream-json] [--json]
sandbox service list	agent-testbench sandbox service list [--store NAME_OR_DSN] [--environment ENV_ID] [--include-components] [--service ID] [--kind KIND] [--status STATUS] [--json]
template-package init	agent-testbench template-package init --output PATH [--id ID] [--display-name NAME] [--force]
template-package install	agent-testbench template-package install --from PATH [--profile-home PATH] [--force]
template-package pack	agent-testbench template-package pack --profile PATH_OR_ID --output PATH [--profile-home PATH] [--force]
template-package list	agent-testbench template-package list [--profile-home PATH] [--json]
template-package inspect	agent-testbench template-package inspect --template-package PATH_OR_ID [--profile-home PATH]
template-package export	agent-testbench template-package export --store NAME_OR_DSN --output PATH [--force] [--json]
template-package catalog list	agent-testbench template-package catalog list [--active] [--store NAME_OR_DSN] [--json]	surface=internal
template-package catalog restore	agent-testbench template-package catalog restore --profile ID [--store NAME_OR_DSN] [--json]	surface=internal
template-package verify	agent-testbench template-package verify --template-package PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--require-case-runs] [--require-workflow-runs] [--json] [--force]
template-package import	agent-testbench template-package import --from PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--audit] [--require-audit-ok] [--force]
executor plan	agent-testbench executor plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--json]	replacement=agent-testbench map explain
evidence import	agent-testbench evidence import --from PATH --profile ID [--store NAME_OR_DSN]
evidence inspect	agent-testbench evidence inspect --view list [--store NAME_OR_DSN] [--run ID] [--json] | agent-testbench evidence inspect --view tasks --run ID [--store NAME_OR_DSN] [--step ID] [--case ID] [--kind KIND] [--status STATUS] [--json]
evidence list	agent-testbench evidence list [--store NAME_OR_DSN] [--run ID] [--json]	replacement=agent-testbench evidence inspect --view list
evidence tasks	agent-testbench evidence tasks [--store NAME_OR_DSN] --run ID [--step ID] [--case ID] [--kind KIND] [--status STATUS] [--json]	replacement=agent-testbench evidence inspect --view tasks
trace topology collect	agent-testbench trace topology collect --run ID [--store NAME_OR_DSN] --trace-graphql-url URL [--step ID] [--case ID] [--request ID] [--endpoint TEXT] [--trace-id ID] [--json]	surface=internal	replacement=agent-testbench evidence inspect --view tasks --run RUN_ID --json
replay evidence	agent-testbench replay evidence --trace-id ID [--json]	surface=internal	replacement=agent-testbench evidence inspect --view list --run RUN_ID --json
workflow discover	agent-testbench workflow discover [--store NAME_OR_DSN] [--filter TEXT] [--service ID] [--json]	replacement=agent-testbench map inspect --view list --json or agent-testbench map inspect --view workflows --map MAP_ID --json
workflow register	agent-testbench workflow register --id ID [--store NAME_OR_DSN] [--profile ID] [--display-name NAME] [--description TEXT] [--base-step-timeout-ms N] [--timeout-offset-ms N] [--audit] [--json]	replacement=agent-testbench map import-workflows --workflow WORKFLOW_ID --map MAP_ID
workflow upsert	agent-testbench workflow upsert --id ID [--store NAME_OR_DSN] [--profile ID] [--display-name NAME] [--description TEXT] [--base-step-timeout-ms N] [--timeout-offset-ms N] [--audit] [--json]	replacement=agent-testbench map import-workflows --workflow WORKFLOW_ID --map MAP_ID
workflow binding register	agent-testbench workflow binding register --workflow ID --step ID --node ID [--case ID] [--store NAME_OR_DSN] [--profile ID] [--required] [--sort-order N] [--audit] [--json]	replacement=agent-testbench map import-workflows --workflow WORKFLOW_ID --map MAP_ID
workflow binding upsert	agent-testbench workflow binding upsert --workflow ID --step ID --node ID [--case ID] [--store NAME_OR_DSN] [--profile ID] [--required] [--sort-order N] [--audit] [--json]	replacement=agent-testbench map import-workflows --workflow WORKFLOW_ID --map MAP_ID
workflow plan	agent-testbench workflow plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] --workflow ID [--json]	replacement=agent-testbench map explain --map MAP_ID --workflow WORKFLOW_ID
workflow audit	agent-testbench workflow audit --workflow ID [--store NAME_OR_DSN] [--json]	replacement=agent-testbench map doctor --map MAP_ID
workflow task run	agent-testbench workflow task run --workflow ID --step STEP=TASK_NAME_OR_ID [--step STEP=TASK_NAME_OR_ID]... [--store NAME_OR_DSN] [--json]	replacement=agent-testbench task run NAME --command COMMAND or agent-testbench map run --plan PLAN_ID --rerun-task TASK_ID
workflow gate	agent-testbench workflow gate --run ID [--store NAME_OR_DSN] [--require-passed] [--require-steps] [--require-evidence] [--json]	surface=default	reason=workflow compatibility: keeps existing workflow gates visible while map-first flows converge
map import-workflows	agent-testbench map import-workflows [--store NAME_OR_DSN] [--map ID] [--workflow ID] [--display-name NAME] [--description TEXT] [--json]
map list	agent-testbench map list [--store NAME_OR_DSN] [--json]	replacement=agent-testbench map inspect --view list
map plans	agent-testbench map plans --map ID [--store NAME_OR_DSN] [--limit N] [--json]	replacement=agent-testbench map inspect --view plans --map MAP_ID
map update	agent-testbench map update --map ID [--display-name NAME] [--description TEXT] [--status STATUS] [--store NAME_OR_DSN] [--json]
map snapshot	agent-testbench map snapshot --map ID --version VERSION [--status STATUS] [--summary TEXT] [--store NAME_OR_DSN] [--json]
map publish	agent-testbench map publish --map ID --version VERSION [--summary TEXT] [--store NAME_OR_DSN] [--json]
map versions	agent-testbench map versions --map ID [--store NAME_OR_DSN] [--json]
map coverage	agent-testbench map coverage --map ID [--store NAME_OR_DSN] [--json]	replacement=agent-testbench map inspect --view coverage --map MAP_ID
map doctor	agent-testbench map doctor --map ID [--store NAME_OR_DSN] [--json]	surface=default	reason=map lifecycle: inspect, plan, execute, gate, and review a test scenario map
map diff	agent-testbench map diff --map ID --from VERSION [--to VERSION|working] [--store NAME_OR_DSN] [--json]
map validation list	agent-testbench map validation list --map ID [--interface ID] [--anchor NODE_OR_CASE_ID] [--store NAME_OR_DSN] [--json]
map validation attach	agent-testbench map validation attach --map ID --anchor NODE_OR_CASE_ID --case CASE_ID [--interface ID] [--store NAME_OR_DSN] [--json]
map workflows	agent-testbench map workflows --map ID [--store NAME_OR_DSN] [--filter TEXT] [--json]	replacement=agent-testbench map inspect --view workflows --map MAP_ID
map inspect	agent-testbench map inspect [--view list|workflows|coverage|plans|plan] [--map ID] [--plan PLAN_ID] [--filter TEXT] [--limit N] [--store NAME_OR_DSN] [--json]	surface=default	reason=map lifecycle: inspect, plan, execute, gate, and review a test scenario map
map explain	agent-testbench map explain --map ID [--scope all|workflows|cases] [--case CASE_ID | --node NODE_ID | --path PATH_ID | --workflow WORKFLOW_ID] [--environment ENV_ID] [--save] [--store NAME_OR_DSN] [--json]	surface=default	reason=map lifecycle: inspect, plan, execute, gate, and review a test scenario map
map gate	agent-testbench map gate --plan PLAN_ID [--store NAME_OR_DSN] [--require-passed] [--require-tasks] [--require-evidence] [--json]	surface=default	reason=map lifecycle: inspect, plan, execute, gate, and review a test scenario map
map run	agent-testbench map run [--map ID | --plan PLAN_ID] [--scope all|workflows|cases] [--case CASE_ID | --node NODE_ID | --path PATH_ID | --workflow WORKFLOW_ID] [--resume | --retry-failed | --skip-passed | --rerun-task TASK_ID] [--environment ENV_ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--store NAME_OR_DSN] [--json]	surface=default	reason=map lifecycle: inspect, plan, execute, gate, and review a test scenario map
map plan inspect	agent-testbench map plan inspect --plan PLAN_ID [--store NAME_OR_DSN] [--json]	replacement=agent-testbench map inspect --view plan --plan PLAN_ID
map atlas	agent-testbench map atlas --map ID [--plan PLAN_ID] [--store NAME_OR_DSN] [--filter TEXT] [--output PATH] [--json]	surface=default	reason=map lifecycle: inspect, plan, execute, gate, and review a test scenario map
gate baseline get	agent-testbench gate baseline get --profile ID --subject ID [--store NAME_OR_DSN]	surface=internal
gate baseline set	agent-testbench gate baseline set --profile ID --subject ID --status STATUS [--required] [--store NAME_OR_DSN]	surface=internal
template render	agent-testbench template render [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] --template ID [--fixture ID]
interface-node discover	agent-testbench interface-node discover [--store NAME_OR_DSN] [--filter TEXT] [--json]
interface-node coverage	agent-testbench interface-node coverage [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--workflow ID] [--json]
interface-node coverage-gaps	agent-testbench interface-node coverage-gaps [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--workflow ID] [--json]
interface-node case audit	agent-testbench interface-node case audit --profile PATH --node ID [--json]
interface-node case draft	agent-testbench interface-node case draft --profile PATH --node ID --case-id ID [--title TEXT] [--case-path PATH] [--method METHOD] [--path PATH] [--tag TAG] [--priority PRIORITY] [--owner OWNER] [--output PATH] [--json]
interface-node case apply	agent-testbench interface-node case apply --profile PATH --file PATH [--json]
interface-node case report	agent-testbench interface-node case report --node ID [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--base-url URL] [--output-dir PATH] [--timeout-seconds N] [--json]
case discover	agent-testbench case discover [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]	surface=default	reason=case lifecycle: discover, run, inspect evidence, and gate API or MQ cases
case suite report	agent-testbench case suite report [--view run|coverage|stability|priority|brief|quality|quality-plan|quality-report|inspect|plan|impact|impact-report] [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--signal TEXT] [--change TEXT] [--limit N] [--stability-limit N] [--action ACTION] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--output-dir PATH] [--timeout-seconds N] [--json]	surface=default	reason=case lifecycle: discover, run, inspect evidence, and gate API or MQ cases
case inspect	agent-testbench case inspect [--view diagnose|evidence|runs|timing] [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--kind KIND] [--max-age-minutes N] [--json]	surface=default	reason=case lifecycle: discover, run, inspect evidence, and gate API or MQ cases
case runs	agent-testbench case runs [--store NAME_OR_DSN] [--run ID] [--json]	replacement=agent-testbench case inspect --view runs
case evidence	agent-testbench case evidence [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--json]	replacement=agent-testbench case inspect --view evidence
case timing	agent-testbench case timing [--store NAME_OR_DSN] [--kind KIND] [--max-age-minutes N] [--json]	replacement=agent-testbench case inspect --view timing
case config upsert	agent-testbench case config upsert --case ID [--store NAME_OR_DSN] [--config-id ID] [--node-id ID] [--method METHOD] [--path PATH] [--body-json JSON] [--header KEY=VALUE]... [--headers-json JSON] [--auth-json JSON] [--signed] [--trace-endpoint URL] [--expected-status CODE]... [--response-contains TEXT]... [--response-not-contains TEXT]... [--json]
case run	agent-testbench case run --case-id ID [--base-url URL] [--override KEY=VALUE] [--evidence-dir PATH] [--store NAME_OR_DSN] [--run-id ID] [--json]	surface=default	reason=case lifecycle: discover, run, inspect evidence, and gate API or MQ cases
case incomplete-batches	agent-testbench case incomplete-batches [--profile PATH_OR_ID] [--store NAME_OR_DSN] [--json]
case diagnose	agent-testbench case diagnose [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--json]	replacement=agent-testbench case inspect --view diagnose
case gate	agent-testbench case gate [--store NAME_OR_DSN] [--run ID] [--require-no-failures] [--require-evidence] [--min-passed N] [--json]	surface=default	reason=case lifecycle: discover, run, inspect evidence, and gate API or MQ cases
serve	agent-testbench serve [--profile PATH_OR_ID] [--profile-home PATH] [--host HOST] [--port PORT] [--store NAME_OR_DSN]
help	agent-testbench help
`

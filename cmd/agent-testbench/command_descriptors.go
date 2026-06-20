package main

import "strings"

type commandDescriptor struct {
	Usage string
}

func commandCatalogDescriptors() []commandDescriptor {
	lines := strings.Split(strings.TrimSpace(commandDescriptorUsageText), "\n")
	out := make([]commandDescriptor, 0, len(lines))
	for _, line := range lines {
		usage := strings.TrimSpace(line)
		if usage == "" {
			continue
		}
		out = append(out, commandDescriptor{Usage: usage})
	}
	return out
}

const commandDescriptorUsageText = `
agent-testbench version
agent-testbench setup [--repo PATH] [--store NAME] [--url DSN | --sqlite PATH] [--build-runtime] [--runtime-only] [--json]
agent-testbench onboard [--repo PATH] [--store NAME] [--url DSN | --sqlite PATH] [--build-runtime] [--install-shell] [--bin-dir PATH] [--smoke none|commands|store] [--json]
agent-testbench status [--deep] [--json]
agent-testbench doctor [--fix] [--deep] [--trace-graphql-url URL] [--json]
agent-testbench update [--repo PATH] [--remote NAME] [--branch NAME] [--release TAG|latest] [--channel main|release] [--check] [--force] [--output PATH] [--json]
agent-testbench commands [--area AREA] [--filter TEXT] [--all] [--json]
agent-testbench completion [bash|zsh]
agent-testbench logs [NAME|list] [-n N] [--json]
agent-testbench task catalog [--filter TEXT] [--json]
agent-testbench task suggest --goal TEXT [--json]
agent-testbench task plan TASK_ID [--map ID] [--environment ENV_ID] [--workspace PATH] [--case-run ID] [--run ID] [--store NAME_OR_DSN] [--json]
agent-testbench task run NAME --command COMMAND [--store NAME_OR_DSN] [--shell] [--notify-file PATH] [--notify-webhook URL] [--json]
agent-testbench task run TASK_ID [--map ID] [--environment ENV_ID] [--workspace PATH] [--case-run ID] [--run ID] [--store NAME_OR_DSN] [--dry-run] [--json]
agent-testbench task schedule NAME --command COMMAND (--interval DURATION | --cron EXPR) [--store NAME_OR_DSN] [--notify-file PATH] [--notify-webhook URL] [--json]
agent-testbench task watch NAME --command COMMAND [--store NAME_OR_DSN] [--interval DURATION] [--limit N] [--until always|success|failure] [--notify-file PATH] [--notify-webhook URL] [--json]
agent-testbench task list [--store NAME_OR_DSN] [--json]
agent-testbench task status NAME [--store NAME_OR_DSN] [--json]
agent-testbench task logs NAME [--store NAME_OR_DSN] [-n N] [--json]
agent-testbench task stop NAME [--store NAME_OR_DSN] [--json]
agent-testbench notify test (--file PATH | --webhook URL) [--message TEXT] [--json]
agent-testbench config path
agent-testbench config show [--json]
agent-testbench config edit
agent-testbench store config set NAME --url postgres://...
agent-testbench store config set NAME --url mysql://...
agent-testbench store config set NAME --url sqlite://PATH
agent-testbench store config list [--json]
agent-testbench store use NAME
agent-testbench store current [--json]
agent-testbench store status [--store NAME_OR_DSN] [--json]
agent-testbench store provision [--store NAME_OR_DSN] [--json]
agent-testbench store upgrade [--store NAME_OR_DSN]
agent-testbench store ddl [--backend postgres|mysql] [--store NAME_OR_DSN]
agent-testbench store copy --from NAME_OR_DSN --to NAME_OR_DSN [--require-environment ENV_ID] [--require-verification-workflow ID] [--require-verified-environment] [--require-min-components N] [--require-min-dependencies N] [--require-min-assets N] [--require-inline-asset-bytes N] [--json]
agent-testbench environment register --id ID [--store NAME_OR_DSN] [--display-name NAME] [--service ID] [--repo SERVICE=PATH] [--branch SERVICE=BRANCH] [--checkout SERVICE=PATH] [--package-repo URL] [--package-branch BRANCH] [--package-ref REF] [--compose-file PATH]... [--compose-generated-file TARGET=SOURCE_FILE]... [--compose-env KEY=VALUE]... [--start-command TEXT] [--health-url URL] [--health-tcp HOST:PORT] [--health-command CMD] [--health-compose-service SERVICE] [--verification-workflow ID] [--json]
agent-testbench environment discover [--store NAME_OR_DSN] [--all] [--json]
agent-testbench environment inspect ENV_ID [--store NAME_OR_DSN] [--json]
agent-testbench environment bootstrap ENV_ID [--store NAME_OR_DSN] [--json]
agent-testbench environment configure ENV_ID --view components|repos|startup-files [--repo SERVICE=URL] [--branch SERVICE=BRANCH] [--repo-ref SERVICE=REF] [--checkout SERVICE=PATH] [--file TARGET=SOURCE_FILE|COMPONENT_GRAPH_JSON] [--store NAME_OR_DSN] [--json]
agent-testbench environment repo set ENV_ID [--repo SERVICE=URL] [--branch SERVICE=BRANCH] [--repo-ref SERVICE=REF] [--checkout SERVICE=PATH] [--store NAME_OR_DSN] [--json]
agent-testbench environment startup-file put ENV_ID --file TARGET=SOURCE_FILE [--store NAME_OR_DSN] [--json]
agent-testbench environment components inspect ENV_ID [--store NAME_OR_DSN] [--json]
agent-testbench environment components replace ENV_ID --file COMPONENT_GRAPH_JSON [--store NAME_OR_DSN] [--json]
agent-testbench environment migration add ENV_ID --edge OWNER:PROVIDER --database DB --version VERSION --file SQL_FILE [--description TEXT] [--precondition column-not-exists:TABLE.COLUMN] [--store NAME_OR_DSN] [--json]
agent-testbench environment migration list ENV_ID [--edge OWNER:PROVIDER] [--database DB] [--store NAME_OR_DSN] [--json]
agent-testbench environment migration plan ENV_ID [--edge OWNER:PROVIDER] [--database DB] [--store NAME_OR_DSN] [--json]
agent-testbench environment migration apply ENV_ID --edge OWNER:PROVIDER --database DB --workspace PATH [--through-version VERSION] [--execute] [--store NAME_OR_DSN] [--output-format text|json|stream-json] [--json]
agent-testbench environment migration baseline ENV_ID --edge OWNER:PROVIDER --database DB --workspace PATH [--through-version VERSION] [--execute] [--store NAME_OR_DSN] [--output-format text|json|stream-json] [--json]
agent-testbench environment restore ENV_ID --workspace PATH [--store NAME_OR_DSN] [--execute] [--pull] [--prepare-repos-only] [--assume-clean-docker] [--use-existing-containers] [--clean-docker-state] [--clean-docker-images] [--allow-destructive-docker-cleanup] [--run-workflow --server-url URL] [--base-url URL] [--workflow-output-dir PATH] [--health-timeout-seconds N] [--output-format text|json|stream-json] [--json]
agent-testbench environment status ENV_ID --workspace PATH [--store NAME_OR_DSN] [--json]
agent-testbench environment stop ENV_ID --workspace PATH [--store NAME_OR_DSN] [--down] [--remove-orphans] [--json]
agent-testbench environment service restart ENV_ID --workspace PATH --service SERVICE_OR_COMPONENT [--store NAME_OR_DSN] [--health-timeout-seconds N] [--json]
agent-testbench environment acceptance start ENV_ID --server-url URL --request-id ID [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--json]
agent-testbench environment acceptance report ENV_ID --server-url URL --run ID [--json]
agent-testbench environment verify ENV_ID --run ID --status STATUS [--evidence-complete] [--topology-complete] [--store NAME_OR_DSN] [--json]
agent-testbench environment publish-verified ENV_ID [--store NAME_OR_DSN] [--json]
agent-testbench runtime mysql endpoints [--include-tables] [--json]
agent-testbench sandbox start [--store NAME_OR_DSN] [--service ID | --workflow ID] [--kind KIND] [--timeout-seconds N] [--dry-run] [--output-format text|json|stream-json] [--json]
agent-testbench sandbox service list [--store NAME_OR_DSN] [--environment ENV_ID] [--include-components] [--service ID] [--kind KIND] [--status STATUS] [--json]
agent-testbench template-package init --output PATH [--id ID] [--display-name NAME] [--force]
agent-testbench template-package install --from PATH [--profile-home PATH] [--force]
agent-testbench template-package pack --profile PATH_OR_ID --output PATH [--profile-home PATH] [--force]
agent-testbench template-package list [--profile-home PATH] [--json]
agent-testbench template-package inspect --template-package PATH_OR_ID [--profile-home PATH]
agent-testbench template-package export --store NAME_OR_DSN --output PATH [--force] [--json]
agent-testbench template-package catalog list [--active] [--store NAME_OR_DSN] [--json]
agent-testbench template-package catalog restore --profile ID [--store NAME_OR_DSN] [--json]
agent-testbench template-package verify --template-package PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--require-case-runs] [--require-workflow-runs] [--json] [--force]
agent-testbench template-package import --from PATH_OR_ID [--profile-home PATH] [--store NAME_OR_DSN] [--json] [--audit] [--require-audit-ok] [--force]
agent-testbench executor plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--json]
agent-testbench evidence import --from PATH --profile ID [--store NAME_OR_DSN]
agent-testbench evidence inspect [--view list|tasks] [--store NAME_OR_DSN] [--run ID] [--step ID] [--case ID] [--kind KIND] [--status STATUS] [--json]
agent-testbench evidence list [--store NAME_OR_DSN] [--run ID] [--json]
agent-testbench evidence tasks [--store NAME_OR_DSN] --run ID [--step ID] [--case ID] [--kind KIND] [--status STATUS] [--json]
agent-testbench trace topology collect --run ID [--store NAME_OR_DSN] --trace-graphql-url URL [--step ID] [--case ID] [--request ID] [--endpoint TEXT] [--trace-id ID] [--json]
agent-testbench replay evidence --trace-id ID [--json]
agent-testbench workflow discover [--store NAME_OR_DSN] [--filter TEXT] [--service ID] [--json]
agent-testbench workflow register --id ID [--store NAME_OR_DSN] [--profile ID] [--display-name NAME] [--description TEXT] [--base-step-timeout-ms N] [--timeout-offset-ms N] [--audit] [--json]
agent-testbench workflow upsert --id ID [--store NAME_OR_DSN] [--profile ID] [--display-name NAME] [--description TEXT] [--base-step-timeout-ms N] [--timeout-offset-ms N] [--audit] [--json]
agent-testbench workflow binding register --workflow ID --step ID --node ID [--case ID] [--store NAME_OR_DSN] [--profile ID] [--required] [--sort-order N] [--audit] [--json]
agent-testbench workflow binding upsert --workflow ID --step ID --node ID [--case ID] [--store NAME_OR_DSN] [--profile ID] [--required] [--sort-order N] [--audit] [--json]
agent-testbench workflow plan [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] --workflow ID [--json]
agent-testbench workflow audit --workflow ID [--store NAME_OR_DSN] [--json]
agent-testbench workflow task run --workflow ID --step STEP=TASK_NAME_OR_ID [--step STEP=TASK_NAME_OR_ID]... [--store NAME_OR_DSN] [--json]
agent-testbench workflow gate --run ID [--store NAME_OR_DSN] [--require-passed] [--require-steps] [--require-evidence] [--json]
agent-testbench map import-workflows [--store NAME_OR_DSN] [--map ID] [--workflow ID] [--display-name NAME] [--description TEXT] [--json]
agent-testbench map list [--store NAME_OR_DSN] [--json]
agent-testbench map plans --map ID [--store NAME_OR_DSN] [--limit N] [--json]
agent-testbench map update --map ID [--display-name NAME] [--description TEXT] [--status STATUS] [--store NAME_OR_DSN] [--json]
agent-testbench map snapshot --map ID --version VERSION [--status STATUS] [--summary TEXT] [--store NAME_OR_DSN] [--json]
agent-testbench map publish --map ID --version VERSION [--summary TEXT] [--store NAME_OR_DSN] [--json]
agent-testbench map versions --map ID [--store NAME_OR_DSN] [--json]
agent-testbench map coverage --map ID [--store NAME_OR_DSN] [--json]
agent-testbench map doctor --map ID [--store NAME_OR_DSN] [--json]
agent-testbench map diff --map ID --from VERSION [--to VERSION|working] [--store NAME_OR_DSN] [--json]
agent-testbench map validation list --map ID [--interface ID] [--anchor NODE_OR_CASE_ID] [--store NAME_OR_DSN] [--json]
agent-testbench map validation attach --map ID --anchor NODE_OR_CASE_ID --case CASE_ID [--interface ID] [--store NAME_OR_DSN] [--json]
agent-testbench map workflows --map ID [--store NAME_OR_DSN] [--filter TEXT] [--json]
agent-testbench map inspect [--view list|workflows|coverage|plans|plan] [--map ID] [--plan PLAN_ID] [--filter TEXT] [--limit N] [--store NAME_OR_DSN] [--json]
agent-testbench map explain --map ID [--scope all|workflows|cases] [--case CASE_ID | --node NODE_ID | --path PATH_ID | --workflow WORKFLOW_ID] [--environment ENV_ID] [--save] [--store NAME_OR_DSN] [--json]
agent-testbench map gate --plan PLAN_ID [--store NAME_OR_DSN] [--require-passed] [--require-tasks] [--require-evidence] [--json]
agent-testbench map run [--map ID | --plan PLAN_ID] [--scope all|workflows|cases] [--case CASE_ID | --node NODE_ID | --path PATH_ID | --workflow WORKFLOW_ID] [--resume | --retry-failed | --skip-passed | --rerun-task TASK_ID] [--environment ENV_ID] [--base-url URL] [--evidence-dir PATH] [--timeout-seconds N] [--store NAME_OR_DSN] [--json]
agent-testbench map plan inspect --plan PLAN_ID [--store NAME_OR_DSN] [--json]
agent-testbench map atlas --map ID [--plan PLAN_ID] [--store NAME_OR_DSN] [--filter TEXT] [--output PATH] [--json]
agent-testbench gate baseline get --profile ID --subject ID [--store NAME_OR_DSN]
agent-testbench gate baseline set --profile ID --subject ID --status STATUS [--required] [--store NAME_OR_DSN]
agent-testbench template render [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] --template ID [--fixture ID]
agent-testbench interface-node discover [--store NAME_OR_DSN] [--filter TEXT] [--json]
agent-testbench interface-node coverage [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--workflow ID] [--json]
agent-testbench interface-node coverage-gaps [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--workflow ID] [--json]
agent-testbench interface-node case audit --profile PATH --node ID [--json]
agent-testbench interface-node case draft --profile PATH --node ID --case-id ID [--title TEXT] [--case-path PATH] [--method METHOD] [--path PATH] [--tag TAG] [--priority PRIORITY] [--owner OWNER] [--output PATH] [--json]
agent-testbench interface-node case apply --profile PATH --file PATH [--json]
agent-testbench interface-node case report --node ID [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--base-url URL] [--output-dir PATH] [--timeout-seconds N] [--json]
agent-testbench case discover [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--json]
agent-testbench case suite report [--view run|coverage|stability|priority|brief|quality|quality-plan|quality-report|inspect|plan|impact|impact-report] [--profile PATH_OR_ID] [--profile-home PATH] [--store NAME_OR_DSN] [--filter TEXT] [--node ID] [--tag TAG] [--status STATUS] [--owner OWNER] [--priority PRIORITY] [--signal TEXT] [--change TEXT] [--limit N] [--stability-limit N] [--action ACTION] [--request-id ID] [--base-url URL] [--evidence-dir PATH] [--output-dir PATH] [--timeout-seconds N] [--json]
agent-testbench case inspect [--view diagnose|evidence|runs|timing] [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--kind KIND] [--max-age-minutes N] [--json]
agent-testbench case runs [--store NAME_OR_DSN] [--run ID] [--json]
agent-testbench case evidence [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--json]
agent-testbench case timing [--store NAME_OR_DSN] [--kind KIND] [--max-age-minutes N] [--json]
agent-testbench case config upsert --case ID [--store NAME_OR_DSN] [--config-id ID] [--node-id ID] [--method METHOD] [--path PATH] [--body-json JSON] [--header KEY=VALUE]... [--headers-json JSON] [--auth-json JSON] [--signed] [--trace-endpoint URL] [--expected-status CODE]... [--response-contains TEXT]... [--response-not-contains TEXT]... [--json]
agent-testbench case run --case-id ID [--base-url URL] [--override KEY=VALUE] [--evidence-dir PATH] [--store NAME_OR_DSN] [--run-id ID] [--json]
agent-testbench case incomplete-batches [--profile PATH_OR_ID] [--store NAME_OR_DSN] [--json]
agent-testbench case diagnose [--store NAME_OR_DSN] [--case-run ID | --run ID [--case-id ID] [--step-id ID]] [--json]
agent-testbench case gate [--store NAME_OR_DSN] [--run ID] [--require-no-failures] [--require-evidence] [--min-passed N] [--json]
agent-testbench serve [--profile PATH_OR_ID] [--profile-home PATH] [--host HOST] [--port PORT] [--store NAME_OR_DSN]
agent-testbench help
`

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type commandCatalogReport struct {
	OK       bool                 `json:"ok"`
	Filter   string               `json:"filter,omitempty"`
	Area     string               `json:"area,omitempty"`
	Tier     string               `json:"tier,omitempty"`
	Audience string               `json:"audience,omitempty"`
	All      bool                 `json:"all,omitempty"`
	Count    int                  `json:"count"`
	Commands []commandCatalogItem `json:"commands"`
}

type commandCatalogItem struct {
	Command     string   `json:"command"`
	Area        string   `json:"area"`
	Path        []string `json:"path"`
	Usage       string   `json:"usage"`
	StoreAware  bool     `json:"storeAware"`
	Tags        []string `json:"tags"`
	Tier        string   `json:"tier"`
	Audience    string   `json:"audience"`
	Stability   string   `json:"stability"`
	Replacement string   `json:"replacement,omitempty"`
	Lifecycle   string   `json:"lifecycle,omitempty"`
	Rank        int      `json:"rank,omitempty"`
}

type commandCatalogOptions struct {
	All      bool
	Tier     string
	Audience string
}

const (
	cliCommandDoctor = "doctor"

	commandCatalogTierDaily      = "daily"
	commandCatalogTierAdvanced   = "advanced"
	commandCatalogTierCompat     = "compat"
	commandCatalogTierDeprecated = "deprecated"

	commandCatalogAudienceAgent     = "agent"
	commandCatalogAudienceOperator  = "operator"
	commandCatalogAudienceDeveloper = "developer"

	commandCatalogStabilityStable = "stable"
	commandCatalogStabilityLegacy = "legacy"

	commandCatalogCaseSuiteCoverage = "case suite coverage"
)

func runCommands(args []string) error {
	flags := flag.NewFlagSet("commands", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	filter := flags.String("filter", "", "Filter command catalog by command, area, usage, or tag")
	area := flags.String("area", "", "Restrict command catalog to one area, such as store, case, workflow, or environment")
	all := flags.Bool("all", false, "Show daily, advanced, compatibility, and deprecated commands")
	tier := flags.String("tier", "", "Restrict command catalog to daily, advanced, compat, or deprecated")
	audience := flags.String("audience", "", "Restrict command catalog to agent, operator, developer, or internal")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable command catalog")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options := commandCatalogOptions{
		All:      *all,
		Tier:     strings.TrimSpace(*tier),
		Audience: strings.TrimSpace(*audience),
	}
	if !options.All && options.Tier == "" {
		options.Tier = commandCatalogTierDaily
	}
	report := commandCatalogForAreaWithOptions(*filter, *area, options)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCommandCatalog(report)
	return nil
}

func commandCatalog(filter string) commandCatalogReport {
	return commandCatalogForArea(filter, "")
}

func commandCatalogForArea(filter string, area string) commandCatalogReport {
	return commandCatalogForAreaWithOptions(filter, area, commandCatalogOptions{Tier: commandCatalogTierDaily})
}

func commandCatalogForAreaWithOptions(filter string, area string, options commandCatalogOptions) commandCatalogReport {
	filter = strings.TrimSpace(filter)
	area = strings.TrimSpace(area)
	options.Tier = strings.TrimSpace(options.Tier)
	options.Audience = strings.TrimSpace(options.Audience)
	report := commandCatalogReport{
		OK:       true,
		Filter:   filter,
		Area:     area,
		Tier:     options.Tier,
		Audience: options.Audience,
		All:      options.All,
		Commands: []commandCatalogItem{},
	}
	seen := map[string]int{}
	for _, usage := range commandUsageLines() {
		item := commandCatalogItemFromUsage(usage)
		if len(item.Path) == 0 {
			continue
		}
		if area != "" && item.Area != area {
			continue
		}
		if options.Tier != "" && item.Tier != options.Tier {
			continue
		}
		if options.Audience != "" && item.Audience != options.Audience {
			continue
		}
		if !commandCatalogMatches(item, filter) {
			continue
		}
		if index, ok := seen[item.Command]; ok {
			report.Commands[index] = preferredCommandCatalogItem(report.Commands[index], item)
			continue
		}
		seen[item.Command] = len(report.Commands)
		report.Commands = append(report.Commands, item)
	}
	sortCommandCatalog(report.Commands, filter)
	report.Count = len(report.Commands)
	return report
}

func commandUsageLines() []string {
	lines := strings.Split(helpText(), "\n")
	out := []string{}
	inUsage := false
	for _, line := range lines {
		usage := strings.TrimSpace(line)
		if usage == "Usage:" {
			inUsage = true
			continue
		}
		if inUsage && usage == "" {
			break
		}
		if !inUsage {
			continue
		}
		if strings.HasPrefix(usage, "agent-testbench ") {
			out = append(out, usage)
		}
	}
	return out
}

func commandCatalogItemFromUsage(usage string) commandCatalogItem {
	rest := strings.TrimSpace(strings.TrimPrefix(usage, "agent-testbench "))
	fields := strings.Fields(rest)
	path := []string{}
	for _, field := range fields {
		if commandUsagePathStops(field) {
			break
		}
		path = append(path, strings.Trim(field, ","))
	}
	area := ""
	if len(path) > 0 {
		area = path[0]
	}
	command := strings.Join(path, " ")
	metadata := commandCatalogMetadata(command, area, usage)
	tags := commandCatalogTags(command, area, usage)
	if metadata.Lifecycle != "" {
		tags = append(tags, metadata.Lifecycle)
	}
	return commandCatalogItem{
		Command:     command,
		Area:        area,
		Path:        path,
		Usage:       usage,
		StoreAware:  strings.Contains(usage, "--store NAME_OR_DSN"),
		Tags:        normalizeStringList(append(tags, metadata.Tier, metadata.Audience, metadata.Stability)),
		Tier:        metadata.Tier,
		Audience:    metadata.Audience,
		Stability:   metadata.Stability,
		Replacement: metadata.Replacement,
		Lifecycle:   metadata.Lifecycle,
		Rank:        metadata.Rank,
	}
}

func preferredCommandCatalogItem(left commandCatalogItem, right commandCatalogItem) commandCatalogItem {
	if commandCatalogTierRank(right.Tier) < commandCatalogTierRank(left.Tier) {
		return right
	}
	if commandCatalogTierRank(right.Tier) > commandCatalogTierRank(left.Tier) {
		return left
	}
	if right.StoreAware && !left.StoreAware {
		return right
	}
	return left
}

func commandCatalogTierRank(tier string) int {
	switch tier {
	case commandCatalogTierDaily:
		return 0
	case commandCatalogTierAdvanced:
		return 1
	case commandCatalogTierCompat:
		return 2
	case commandCatalogTierDeprecated:
		return 3
	default:
		return 4
	}
}

type commandCatalogMetadataReport struct {
	Tier        string
	Audience    string
	Stability   string
	Replacement string
	Lifecycle   string
	Rank        int
}

func commandCatalogMetadata(command string, area string, usage string) commandCatalogMetadataReport {
	metadata := commandCatalogMetadataReport{Tier: commandCatalogTierAdvanced, Audience: commandCatalogAudienceOperator, Stability: commandCatalogStabilityStable}
	if area == "map" {
		metadata.Audience = commandCatalogAudienceAgent
		metadata.Lifecycle = commandCatalogMapLifecycle(command)
		metadata.Rank = commandCatalogTaskRank(command)
	}
	if strings.Contains(usage, "--offline-template-package") || strings.Contains(usage, "--case PATH") {
		metadata.Tier = commandCatalogTierCompat
		metadata.Audience = commandCatalogAudienceAgent
		metadata.Stability = commandCatalogStabilityLegacy
		return metadata
	}
	if commandCatalogDailyCommands()[command] {
		metadata.Tier = commandCatalogTierDaily
		metadata.Audience = commandCatalogAudienceAgent
		return metadata
	}
	if replacement, ok := commandCatalogCompatReplacements()[command]; ok {
		metadata.Tier = commandCatalogTierCompat
		metadata.Audience = commandCatalogAudienceAgent
		metadata.Stability = commandCatalogStabilityLegacy
		metadata.Replacement = replacement
		return metadata
	}
	if replacement, ok := commandCatalogAdvancedReplacements()[command]; ok {
		metadata.Replacement = replacement
	}
	if area == "profile" || area == "template-package" || area == "runtime" || area == "executor" || area == "trace" || area == "replay" {
		metadata.Audience = commandCatalogAudienceDeveloper
	}
	if area == "completion" || command == "notify test" || command == "logs" || command == "config edit" {
		metadata.Audience = commandCatalogAudienceOperator
	}
	return metadata
}

func commandCatalogMapLifecycle(command string) string {
	switch command {
	case "map list", "map workflows", "map coverage", "map plans", "map versions":
		return "inspect"
	case "map import-workflows", "map doctor", "map diff", "map validation list", "map validation attach", "map update", "map snapshot", "map publish":
		return "maintain"
	case "map explain", "map plan inspect", "map run explain":
		return "plan"
	case "map run", "map gate":
		return "execute"
	case "map atlas":
		return "review"
	default:
		return ""
	}
}

func commandCatalogTaskRank(command string) int {
	switch command {
	case "map doctor":
		return 10
	case "map coverage":
		return 20
	case "map diff":
		return 30
	case "map validation list":
		return 40
	case "map validation attach":
		return 50
	case "map update":
		return 60
	case "map snapshot":
		return 70
	case "map publish":
		return 80
	case "map versions":
		return 90
	case "map import-workflows":
		return 100
	case "map list":
		return 110
	case "map workflows":
		return 120
	case "map explain":
		return 210
	case "map plan inspect":
		return 220
	case "map run":
		return 230
	case "map gate":
		return 240
	case "map run explain":
		return 250
	case "map plans":
		return 260
	case "map atlas":
		return 310
	default:
		return 0
	}
}

func sortCommandCatalog(commands []commandCatalogItem, filter string) {
	needle := normalizedDiscoveryText(filter)
	if needle == "" {
		return
	}
	if !strings.Contains(needle, "maintainmap") && !strings.Contains(needle, "executemap") {
		return
	}
	sort.SliceStable(commands, func(i, j int) bool {
		left := commandCatalogSortRank(commands[i])
		right := commandCatalogSortRank(commands[j])
		if left != right {
			return left < right
		}
		return commands[i].Command < commands[j].Command
	})
}

func commandCatalogSortRank(item commandCatalogItem) int {
	if item.Rank > 0 {
		return item.Rank
	}
	return 100000
}

func commandCatalogDailyCommands() map[string]bool {
	return map[string]bool{
		"status":                      true,
		cliCommandDoctor:              true,
		"commands":                    true,
		"store current":               true,
		"store status":                true,
		"environment discover":        true,
		"environment inspect":         true,
		"environment restore":         true,
		"environment status":          true,
		"environment stop":            true,
		"environment service restart": true,
		"map list":                    true,
		"map coverage":                true,
		"map doctor":                  true,
		"map explain":                 true,
		"map gate":                    true,
		"map run":                     true,
		"map atlas":                   true,
		"case discover":               true,
		"case suite report":           true,
		"case runs":                   true,
		"case evidence":               true,
		"case diagnose":               true,
		"case gate":                   true,
		"case run":                    true,
		"workflow gate":               true,
		"task catalog":                true,
		"task suggest":                true,
		"task plan":                   true,
		"task run":                    true,
	}
}

func commandCatalogCompatReplacements() map[string]string {
	return map[string]string{
		commandCatalogCaseSuiteCoverage: "agent-testbench case suite report --view coverage",
		"case suite stability":          "agent-testbench case suite report --view stability",
		"case suite priority":           "agent-testbench case suite report --view priority",
		"case suite brief":              "agent-testbench case suite report --view brief",
		"case suite quality":            "agent-testbench case suite report --view quality",
		"case suite quality-plan":       "agent-testbench case suite report --view quality-plan",
		"case suite quality-report":     "agent-testbench case suite report --view quality-report",
		"case suite impact":             "agent-testbench case suite report --view impact",
		"case suite impact-report":      "agent-testbench case suite report --view impact-report",
		"workflow acceptance start":     "agent-testbench environment acceptance start",
		"workflow acceptance report":    "agent-testbench environment acceptance report",
		"baseline get":                  "agent-testbench gate baseline get",
		"baseline set":                  "agent-testbench gate baseline set",
		"map run explain":               "agent-testbench map plan inspect",
	}
}

func commandCatalogAdvancedReplacements() map[string]string {
	return map[string]string{
		"executor plan":              "agent-testbench map explain",
		"runtime mysql endpoints":    "agent-testbench store status --json",
		"trace topology collect":     "agent-testbench evidence tasks --run RUN_ID --json",
		"replay evidence":            "agent-testbench evidence list --run RUN_ID --json",
		"sandbox service register":   "agent-testbench environment restore or agent-testbench environment service restart",
		"sandbox interface register": "agent-testbench case config upsert",
	}
}

func commandUsagePathStops(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || strings.HasPrefix(token, "[") || strings.HasPrefix(token, "(") || strings.HasPrefix(token, "--") || strings.Contains(token, "|") {
		return true
	}
	trimmed := strings.Trim(token, ".,")
	if strings.Contains(trimmed, "=") || strings.Contains(trimmed, ":") || strings.Contains(trimmed, "/") {
		return true
	}
	hasLetter := false
	for _, item := range trimmed {
		if item >= 'a' && item <= 'z' {
			return false
		}
		if item >= 'A' && item <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
}

func commandCatalogTags(command string, area string, usage string) []string {
	tags := []string{area}
	if strings.Contains(usage, "--store NAME_OR_DSN") {
		tags = append(tags, "store-first")
	}
	if strings.Contains(usage, "--json") {
		tags = append(tags, "json")
	}
	if strings.Contains(usage, "gate") || strings.Contains(usage, "verify") || strings.Contains(usage, "acceptance") {
		tags = append(tags, "quality-gate")
	}
	if strings.Contains(usage, "diagnose") || strings.Contains(usage, "evidence") || strings.Contains(usage, "trace") {
		tags = append(tags, "evidence")
	}
	if strings.Contains(usage, "workflow") {
		tags = append(tags, "workflow")
	}
	tags = append(tags, commandCatalogTaskTags(command)...)
	return normalizeStringList(tags)
}

func commandCatalogTaskTags(command string) []string {
	switch command {
	case "map import-workflows", "map list", "map coverage", "map doctor", "map workflows", "map atlas",
		"map update", "map snapshot", "map publish", "map versions", "map diff", "map validation list", "map validation attach":
		return []string{"maintain map", "map maintenance"}
	case "map plans", "map explain", "map gate", "map run", "map plan inspect", "map run explain":
		return []string{"execute map", "map execution"}
	case "environment restore", "environment status", "environment stop", "environment service restart", "environment discover", "environment inspect":
		return []string{"restore environment", "environment operations"}
	case "case diagnose", "case evidence", "case gate", "workflow gate", "evidence list", "evidence tasks", cliCommandDoctor:
		return []string{"diagnose evidence", "evidence diagnosis"}
	default:
		return nil
	}
}

func commandCatalogMatches(item commandCatalogItem, filter string) bool {
	if filter == "" {
		return true
	}
	needle := normalizedDiscoveryText(filter)
	haystack := normalizedDiscoveryText(strings.Join(append([]string{item.Command, item.Area, item.Usage}, item.Tags...), " "))
	if item.Replacement != "" {
		haystack += " " + normalizedDiscoveryText(item.Replacement)
	}
	return strings.Contains(haystack, needle)
}

func printCommandCatalog(report commandCatalogReport) {
	fmt.Println("Commands")
	fmt.Printf("Total: %d\n", report.Count)
	if report.Filter != "" {
		fmt.Printf("Filter: %s\n", report.Filter)
	}
	if report.Area != "" {
		fmt.Printf("Area: %s\n", report.Area)
	}
	for _, item := range report.Commands {
		fmt.Printf("- %s [%s]\n", item.Command, item.Area)
		fmt.Printf("  Tier: %s Audience: %s Stability: %s\n", item.Tier, item.Audience, item.Stability)
		if item.Lifecycle != "" {
			fmt.Printf("  Lifecycle: %s\n", item.Lifecycle)
		}
		if item.Replacement != "" {
			fmt.Printf("  Replacement: %s\n", item.Replacement)
		}
		fmt.Printf("  %s\n", item.Usage)
	}
}

func commandHelpText(prefix []string) (string, error) {
	prefix = normalizeCommandHelpPrefix(prefix)
	if len(prefix) == 0 {
		return helpText(), nil
	}
	command := strings.Join(prefix, " ")
	if usages := commandUsageLinesForCommand(command); len(usages) > 0 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "Command: %s\n\nUsage:\n", command)
		for _, usage := range usages {
			fmt.Fprintf(&builder, "  %s\n", usage)
		}
		return strings.TrimRight(builder.String(), "\n"), nil
	}

	report := commandCatalogForAreaWithOptions("", "", commandCatalogOptions{All: true})
	matches := []commandCatalogItem{}
	for _, item := range report.Commands {
		if commandPathHasPrefix(item.Path, prefix) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("unknown help target: %s", command)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "Commands: %s\n\nUsage:\n", command)
	if command == "map" && len(prefix) == 1 {
		appendMapLifecycleHelp(&builder, matches)
	} else {
		for _, item := range matches {
			fmt.Fprintf(&builder, "  %s\n", item.Usage)
		}
	}
	fmt.Fprintf(&builder, "\nUse `agent-testbench commands --filter %q --all` for machine-readable metadata.", command)
	return strings.TrimRight(builder.String(), "\n"), nil
}

func appendMapLifecycleHelp(builder *strings.Builder, matches []commandCatalogItem) {
	byLifecycle := map[string][]commandCatalogItem{}
	for _, item := range matches {
		lifecycle := item.Lifecycle
		if lifecycle == "" {
			lifecycle = "other"
		}
		byLifecycle[lifecycle] = append(byLifecycle[lifecycle], item)
	}
	for _, group := range commandCatalogMapLifecycleGroups() {
		items := byLifecycle[group.ID]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			left := commandCatalogSortRank(items[i])
			right := commandCatalogSortRank(items[j])
			if left != right {
				return left < right
			}
			return items[i].Command < items[j].Command
		})
		fmt.Fprintf(builder, "\n%s:\n", group.Label)
		for _, item := range items {
			fmt.Fprintf(builder, "  %s\n", item.Usage)
		}
	}
}

type commandCatalogLifecycleGroup struct {
	ID    string
	Label string
}

func commandCatalogMapLifecycleGroups() []commandCatalogLifecycleGroup {
	return []commandCatalogLifecycleGroup{
		{ID: "inspect", Label: "Inspect"},
		{ID: "maintain", Label: "Maintain"},
		{ID: "plan", Label: "Plan"},
		{ID: "execute", Label: "Execute"},
		{ID: "review", Label: "Review"},
	}
}

func printCommandHelp(prefix []string) error {
	text, err := commandHelpText(prefix)
	if err != nil {
		return err
	}
	fmt.Println(text)
	return nil
}

func commandUsageLinesForCommand(command string) []string {
	lines := []string{}
	for _, usage := range commandUsageLines() {
		item := commandCatalogItemFromUsage(usage)
		if item.Command == command {
			lines = append(lines, usage)
		}
	}
	return lines
}

func commandPathHasPrefix(path []string, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for index := range prefix {
		if path[index] != prefix[index] {
			return false
		}
	}
	return true
}

func normalizeCommandHelpPrefix(prefix []string) []string {
	out := []string{}
	for _, item := range prefix {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "-") {
			break
		}
		out = append(out, item)
	}
	return out
}

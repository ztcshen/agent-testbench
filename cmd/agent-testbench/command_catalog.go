package main

import (
	"flag"
	"fmt"
	"os"
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
	tags := commandCatalogTags(area, usage)
	metadata := commandCatalogMetadata(strings.Join(path, " "), area, usage)
	return commandCatalogItem{
		Command:     strings.Join(path, " "),
		Area:        area,
		Path:        path,
		Usage:       usage,
		StoreAware:  strings.Contains(usage, "--store NAME_OR_DSN"),
		Tags:        normalizeStringList(append(tags, metadata.Tier, metadata.Audience, metadata.Stability)),
		Tier:        metadata.Tier,
		Audience:    metadata.Audience,
		Stability:   metadata.Stability,
		Replacement: metadata.Replacement,
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
}

func commandCatalogMetadata(command string, area string, usage string) commandCatalogMetadataReport {
	metadata := commandCatalogMetadataReport{Tier: commandCatalogTierAdvanced, Audience: commandCatalogAudienceOperator, Stability: commandCatalogStabilityStable}
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
	if area == "map" {
		metadata.Audience = commandCatalogAudienceAgent
	}
	if area == "profile" || area == "template-package" || area == "runtime" || area == "executor" || area == "trace" || area == "replay" {
		metadata.Audience = commandCatalogAudienceDeveloper
	}
	if area == "completion" || command == "notify test" || command == "logs" || command == "config edit" {
		metadata.Audience = commandCatalogAudienceOperator
	}
	return metadata
}

func commandCatalogDailyCommands() map[string]bool {
	return map[string]bool{
		"status":                        true,
		cliCommandDoctor:                true,
		"commands":                      true,
		"store current":                 true,
		"store status":                  true,
		"environment discover":          true,
		"environment inspect":           true,
		"environment restore":           true,
		"environment status":            true,
		"environment stop":              true,
		"environment service restart":   true,
		"environment acceptance start":  true,
		"environment acceptance report": true,
		"map list":                      true,
		"map plans":                     true,
		"map coverage":                  true,
		"map doctor":                    true,
		"map workflows":                 true,
		"map explain":                   true,
		"map gate":                      true,
		"map run":                       true,
		"map plan inspect":              true,
		"map atlas":                     true,
		"case discover":                 true,
		"case suite report":             true,
		"case suite inspect":            true,
		"case suite plan":               true,
		"case runs":                     true,
		"case evidence":                 true,
		"case diagnose":                 true,
		"case gate":                     true,
		"case run":                      true,
		"workflow discover":             true,
		"workflow runs":                 true,
		"workflow run":                  true,
		"workflow gate":                 true,
		"workflow report":               true,
		"task run":                      true,
		"gate baseline get":             true,
		"gate baseline set":             true,
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

func commandCatalogTags(area string, usage string) []string {
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
	return normalizeStringList(tags)
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
		if item.Replacement != "" {
			fmt.Printf("  Replacement: %s\n", item.Replacement)
		}
		fmt.Printf("  %s\n", item.Usage)
	}
}

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
	All      bool                 `json:"all,omitempty"`
	Internal bool                 `json:"internal,omitempty"`
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
	Replacement string   `json:"replacement,omitempty"`
	Lifecycle   string   `json:"lifecycle,omitempty"`
	Rank        int      `json:"rank,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	surface     string
}

type commandCatalogOptions struct {
	All      bool
	Internal bool
}

func runCommands(args []string) error {
	flags := flag.NewFlagSet("commands", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	filter := flags.String("filter", "", "Filter command catalog by command, area, usage, or tag")
	area := flags.String("area", "", "Restrict command catalog to one area, such as store, case, workflow, or environment")
	all := flags.Bool("all", false, "Show the full command catalog beyond the default surface")
	internal := flags.Bool("internal", false, "Include internal maintenance and diagnostics commands; use with --all")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable command catalog")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options := commandCatalogOptions{All: *all, Internal: *internal}
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
	return commandCatalogForAreaWithOptions(filter, area, commandCatalogOptions{})
}

func commandCatalogForAreaWithOptions(filter string, area string, options commandCatalogOptions) commandCatalogReport {
	filter = strings.TrimSpace(filter)
	area = strings.TrimSpace(area)
	report := commandCatalogReport{
		OK:       true,
		Filter:   filter,
		Area:     area,
		All:      options.All,
		Internal: options.Internal,
		Commands: []commandCatalogItem{},
	}
	seen := map[string]int{}
	for _, descriptor := range commandCatalogDescriptors() {
		item := commandCatalogItemFromDescriptor(descriptor)
		if len(item.Path) == 0 {
			continue
		}
		if area != "" && item.Area != area {
			continue
		}
		if item.surface == commandCatalogSurfaceInternal && (!options.All || !options.Internal) {
			continue
		}
		if !options.All && item.surface != commandCatalogSurfaceDefault {
			continue
		}
		if options.All {
			item.Reason = ""
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
	descriptors := commandCatalogDescriptors()
	out := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		out = append(out, descriptor.Usage)
	}
	return out
}

func commandCatalogItemFromDescriptor(descriptor commandDescriptor) commandCatalogItem {
	usage := descriptor.Usage
	command := strings.TrimSpace(descriptor.Command)
	path := strings.Fields(command)
	area := ""
	if len(path) > 0 {
		area = path[0]
	}
	metadata := commandCatalogMetadata(descriptor, area)
	tags := commandCatalogTags(command, area, usage)
	if metadata.Lifecycle != "" {
		tags = append(tags, metadata.Lifecycle)
	}
	if metadata.Surface == commandCatalogSurfaceInternal {
		tags = append(tags, commandCatalogSurfaceInternal)
	}
	return commandCatalogItem{
		Command:     command,
		Area:        area,
		Path:        path,
		Usage:       usage,
		StoreAware:  strings.Contains(usage, "--store NAME_OR_DSN"),
		Tags:        normalizeStringList(tags),
		Replacement: metadata.Replacement,
		Lifecycle:   metadata.Lifecycle,
		Rank:        metadata.Rank,
		Reason:      metadata.Reason,
		surface:     metadata.Surface,
	}
}

func preferredCommandCatalogItem(left commandCatalogItem, right commandCatalogItem) commandCatalogItem {
	if commandCatalogSurfaceRank(right.surface) < commandCatalogSurfaceRank(left.surface) {
		return right
	}
	if commandCatalogSurfaceRank(right.surface) > commandCatalogSurfaceRank(left.surface) {
		return left
	}
	if right.StoreAware && !left.StoreAware {
		return right
	}
	return left
}

func commandCatalogSurfaceRank(surface string) int {
	switch surface {
	case commandCatalogSurfaceDefault:
		return 0
	case commandCatalogSurfaceExtended:
		return 1
	case commandCatalogSurfaceInternal:
		return 2
	case commandCatalogSurfaceCompatibility:
		return 3
	case commandCatalogSurfaceDeprecated:
		return 4
	default:
		return 5
	}
}

type commandCatalogMetadataReport struct {
	Surface     string
	Replacement string
	Lifecycle   string
	Rank        int
	Reason      string
}

func commandCatalogMetadata(descriptor commandDescriptor, area string) commandCatalogMetadataReport {
	command := descriptor.Command
	usage := descriptor.Usage
	metadata := commandCatalogMetadataReport{Surface: commandCatalogSurfaceExtended}
	if area == "map" {
		metadata.Lifecycle = commandCatalogMapLifecycle(command)
		metadata.Rank = commandCatalogTaskRank(command)
	}
	if strings.Contains(usage, "--offline-template-package") || strings.Contains(usage, "--case PATH") {
		metadata.Surface = commandCatalogSurfaceCompatibility
		return metadata
	}
	if descriptor.Surface != "" {
		metadata.Surface = descriptor.Surface
	}
	if descriptor.Replacement != "" {
		metadata.Replacement = descriptor.Replacement
	}
	if descriptor.Reason != "" {
		metadata.Reason = descriptor.Reason
	}
	if metadata.Surface == commandCatalogSurfaceInternal {
		return metadata
	}
	return metadata
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
	case commandCatalogMapImportWorkflows, commandCatalogMapInspect, commandCatalogMapList, commandCatalogMapCoverage, commandCatalogMapDoctor, commandCatalogMapWorkflows, commandCatalogMapAtlas,
		commandCatalogMapUpdate, commandCatalogMapSnapshot, commandCatalogMapPublish, commandCatalogMapVersions, commandCatalogMapDiff, commandCatalogMapValidationList, commandCatalogMapValidationAttach, commandCatalogMapValidationPromote:
		return []string{"maintain map", "map maintenance"}
	case commandCatalogMapPlans, commandCatalogMapExplain, commandCatalogMapGate, commandCatalogMapRun, commandCatalogMapPlanInspect:
		return []string{"execute map", "map execution"}
	case "environment restore", "environment status", "environment stop", "environment service restart", "environment discover", "environment inspect":
		return []string{"restore environment", "environment operations"}
	case commandCatalogCaseInspect, "case diagnose", "case evidence", "case gate", "workflow gate", commandCatalogEvidenceInspect, commandCatalogEvidenceList, commandCatalogEvidenceTasks, cliCommandDoctor:
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
	if report.Internal {
		fmt.Println("Internal: true")
	}
	for _, item := range report.Commands {
		fmt.Printf("- %s [%s]\n", item.Command, item.Area)
		if item.Lifecycle != "" {
			fmt.Printf("  Lifecycle: %s\n", item.Lifecycle)
		}
		if item.Reason != "" {
			fmt.Printf("  Reason: %s\n", item.Reason)
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
		return fullHelpText(), nil
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
	matches = commandParentNavigationItems(command, prefix, matches)
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

func commandParentNavigationItems(command string, prefix []string, matches []commandCatalogItem) []commandCatalogItem {
	if len(prefix) != 1 {
		return matches
	}
	switch command {
	case "case", "environment", "map":
		return commandParentDefaultItems(matches)
	case builtInTaskStepEvidence:
		visible := map[string]bool{
			"evidence import":             true,
			commandCatalogEvidenceInspect: true,
		}
		filtered := make([]commandCatalogItem, 0, len(matches))
		for _, item := range matches {
			if visible[item.Command] {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
		return matches
	default:
		return matches
	}
}

func commandParentDefaultItems(matches []commandCatalogItem) []commandCatalogItem {
	defaults := make([]commandCatalogItem, 0, len(matches))
	for _, item := range matches {
		if item.surface == commandCatalogSurfaceDefault {
			defaults = append(defaults, item)
		}
	}
	if len(defaults) == 0 {
		return matches
	}
	return defaults
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
		{ID: commandCatalogLifecycleInspect, Label: "Inspect"},
		{ID: commandCatalogLifecycleMaintain, Label: "Maintain"},
		{ID: commandCatalogLifecyclePlan, Label: "Plan"},
		{ID: commandCatalogLifecycleExecute, Label: "Execute"},
		{ID: commandCatalogLifecycleReview, Label: "Review"},
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
	for _, descriptor := range commandCatalogDescriptors() {
		item := commandCatalogItemFromDescriptor(descriptor)
		if item.Command == command {
			lines = append(lines, descriptor.Usage)
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

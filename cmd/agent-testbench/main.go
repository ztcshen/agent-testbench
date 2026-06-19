package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

const version = "0.1.0"
const interfaceNodeCommand = "interface-node"
const cliCommandTask = "task"

var buildRevision = ""

type versionCommandReport struct {
	Version       string `json:"version"`
	BuildRevision string `json:"buildRevision,omitempty"`
}

type rootCommand func([]string) error

type unknownRootCommandError string

func (e unknownRootCommandError) Error() string {
	return "unknown command: " + string(e)
}

var rootCommands = map[string]rootCommand{
	"commands":           runCommands,
	"setup":              func(args []string) error { return runSetup(context.Background(), args) },
	"onboard":            func(args []string) error { return runOnboard(context.Background(), args) },
	"status":             func(args []string) error { return runStatus(context.Background(), args) },
	cliCommandDoctor:     func(args []string) error { return runDoctor(context.Background(), args) },
	"update":             func(args []string) error { return runUpdate(context.Background(), args) },
	"completion":         runCompletion,
	"logs":               func(args []string) error { return runLogs(context.Background(), args) },
	cliCommandTask:       func(args []string) error { return runTask(context.Background(), args) },
	"watch":              func(args []string) error { return runWatch(context.Background(), args) },
	"notify":             func(args []string) error { return runNotify(context.Background(), args) },
	"store":              func(args []string) error { return runStore(context.Background(), args) },
	"sandbox":            func(args []string) error { return runSandbox(context.Background(), args) },
	"environment":        func(args []string) error { return runEnvironment(context.Background(), args) },
	"runtime":            func(args []string) error { return runRuntime(context.Background(), args) },
	"profile":            runProfile,
	"template-package":   runTemplatePackage,
	"template-packages":  runTemplatePackage,
	"config":             func(args []string) error { return runConfig(context.Background(), args) },
	"evidence":           func(args []string) error { return runEvidence(context.Background(), args) },
	"trace":              func(args []string) error { return runTrace(context.Background(), args) },
	"replay":             runReplay,
	"executor":           func(args []string) error { return runExecutor(context.Background(), args) },
	"workflow":           runWorkflow,
	"map":                func(args []string) error { return runMap(context.Background(), args) },
	"gate":               func(args []string) error { return runGate(context.Background(), args) },
	"baseline":           func(args []string) error { return runBaseline(context.Background(), args) },
	"template":           runTemplate,
	"case":               func(args []string) error { return runCase(context.Background(), args) },
	interfaceNodeCommand: runInterfaceNode,
	"serve":              runServe,
}

func main() {
	if err := runRootCommand(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var unknown unknownRootCommandError
		if errors.As(err, &unknown) {
			printHelp()
		}
		os.Exit(2)
	}
}

func runRootCommand(args []string) error {
	if len(args) < 1 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "version", "--version", "-v":
		return runVersion(args[1:])
	case "--help", "-h":
		printHelp()
		return nil
	case "help":
		if len(args) > 1 {
			return printCommandHelp(args[1:])
		}
		printHelp()
		return nil
	}
	if prefix, ok := commandHelpPrefix(args); ok {
		return printCommandHelp(prefix)
	}
	command, ok := rootCommands[args[0]]
	if !ok {
		return unknownRootCommandError(args[0])
	}
	return command(args[1:])
}

func commandHelpPrefix(args []string) ([]string, bool) {
	if len(args) < 2 {
		return nil, false
	}
	last := args[len(args)-1]
	if last != "--help" && last != "-h" {
		return nil, false
	}
	return args[:len(args)-1], true
}

func runVersion(args []string) error {
	if len(args) == 1 && args[0] == "--json" {
		return writeIndentedJSON(versionCommandReport{Version: version, BuildRevision: strings.TrimSpace(buildRevision)})
	}
	if len(args) != 0 {
		return fmt.Errorf("unexpected version arguments: %s", strings.Join(args, " "))
	}
	fmt.Printf("AgentTestBench %s\n", version)
	return nil
}

func printHelp() {
	fmt.Println(helpText())
}

func helpText() string {
	return helpTextContent
}

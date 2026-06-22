package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type logsListReport struct {
	OK   bool            `json:"ok"`
	Logs []logsListEntry `json:"logs"`
}

type logsListEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type logsTailReport struct {
	OK    bool     `json:"ok"`
	Name  string   `json:"name"`
	Path  string   `json:"path"`
	Lines []string `json:"lines"`
}

func runCompletion(args []string) error {
	shell := "bash"
	if len(args) > 1 {
		return fmt.Errorf("unexpected completion arguments: %s", strings.Join(args[1:], " "))
	}
	if len(args) == 1 {
		shell = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch shell {
	case "bash":
		fmt.Print(bashCompletionScript())
	case "zsh":
		fmt.Print(zshCompletionScript())
	default:
		return fmt.Errorf("unsupported completion shell %q; use bash or zsh", shell)
	}
	return nil
}

func bashCompletionScript() string {
	return fmt.Sprintf("# agent-testbench bash completion\ncomplete -W %q agent-testbench\n", strings.Join(rootCommandNames(), " "))
}

func zshCompletionScript() string {
	commands := strings.Join(rootCommandNames(), " ")
	return "#compdef agent-testbench\n_agent_testbench() {\n  _arguments '1:command:(" + commands + ")'\n}\ncompdef _agent_testbench agent-testbench\n"
}

func rootCommandNames() []string {
	seen := map[string]bool{
		"help":    true,
		"version": true,
	}
	for _, descriptor := range commandCatalogDescriptors() {
		fields := strings.Fields(descriptor.Command)
		if len(fields) > 0 {
			seen[strings.Trim(fields[0], "[]")] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func runLogs(ctx context.Context, args []string) error {
	_ = ctx
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	lines := flags.Int("n", 80, "Number of log lines to print")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable logs report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("unexpected logs arguments: %s", strings.Join(flags.Args()[1:], " "))
	}
	repo, err := resolveUpdateRepo("")
	if err != nil {
		return err
	}
	logDir := filepath.Join(repo, ".runtime", "logs")
	if flags.NArg() == 0 || strings.TrimSpace(flags.Arg(0)) == cliCommandList {
		report, reportErr := logsListReportForDir(logDir)
		if reportErr != nil {
			return reportErr
		}
		if *jsonOutput {
			return writeIndentedJSON(report)
		}
		printLogsList(report)
		return nil
	}
	report, err := logsTailReportForName(logDir, flags.Arg(0), *lines)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printLogsTail(report)
	return nil
}

func logsListReportForDir(logDir string) (logsListReport, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return logsListReport{OK: true, Logs: []logsListEntry{}}, nil
		}
		return logsListReport{}, err
	}
	logs := []logsListEntry{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return logsListReport{}, infoErr
		}
		name := strings.TrimSuffix(entry.Name(), ".log")
		logs = append(logs, logsListEntry{Name: name, Path: filepath.Join(logDir, entry.Name()), Size: info.Size()})
	}
	sort.Slice(logs, func(i int, j int) bool {
		return logs[i].Name < logs[j].Name
	})
	return logsListReport{OK: true, Logs: logs}, nil
}

func logsTailReportForName(logDir string, name string, lineCount int) (logsTailReport, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return logsTailReport{}, fmt.Errorf("log name is required")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return logsTailReport{}, fmt.Errorf("log name must not contain path separators")
	}
	fileName := name
	if !strings.HasSuffix(fileName, ".log") {
		fileName += ".log"
	}
	displayName := strings.TrimSuffix(fileName, ".log")
	path := filepath.Join(logDir, fileName)
	lines, err := tailLogLinesFromFile(path, lineCount)
	if err != nil {
		return logsTailReport{}, err
	}
	return logsTailReport{OK: true, Name: displayName, Path: path, Lines: lines}, nil
}

const logTailReadChunkSize int64 = 32 * 1024

func tailLogLinesFromFile(path string, lineCount int) (lines []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return []string{}, nil
	}
	if lineCount <= 0 {
		return allLogLinesFromFile(file)
	}
	offset := info.Size()
	suffix := []byte{}
	for offset > 0 {
		readSize := logTailReadChunkSize
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		chunk := make([]byte, readSize, int64(len(suffix))+readSize)
		if _, err := file.ReadAt(chunk, offset); err != nil && !errorsIsEOF(err) {
			return nil, err
		}
		suffix = append(chunk, suffix...)
		if bytes.Count(suffix, []byte{'\n'}) >= lineCount {
			break
		}
	}
	return tailLogLines(string(suffix), lineCount), nil
}

func allLogLinesFromFile(file *os.File) ([]string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	raw := strings.Builder{}
	buf := make([]byte, logTailReadChunkSize)
	for {
		read, err := file.Read(buf)
		if read > 0 {
			raw.Write(buf[:read])
		}
		if err == nil {
			continue
		}
		if errorsIsEOF(err) {
			return tailLogLines(raw.String(), 0), nil
		}
		return nil, err
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}

func tailLogLines(raw string, lineCount int) []string {
	raw = strings.TrimRight(raw, "\r\n")
	if raw == "" {
		return []string{}
	}
	lines := strings.Split(raw, "\n")
	if lineCount <= 0 || lineCount >= len(lines) {
		return lines
	}
	return lines[len(lines)-lineCount:]
}

func printLogsList(report logsListReport) {
	fmt.Println("AgentTestBench Logs")
	for _, item := range report.Logs {
		fmt.Printf("- %s\t%s\t%d bytes\n", item.Name, item.Path, item.Size)
	}
}

func printLogsTail(report logsTailReport) {
	for _, line := range report.Lines {
		fmt.Println(line)
	}
}

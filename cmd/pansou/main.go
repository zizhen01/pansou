package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"pansou/internal/app"
	"pansou/internal/core"
	"pansou/internal/output"
	"pansou/internal/sources/legacy"
	"pansou/internal/sources/tg"
	"sort"
	"strings"
	"time"
)

const defaultChannels = "Aliyun_4K_Movies,Quark_Movies"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "recent" {
		return runRecent(args[1:], stdout, stderr)
	}
	if len(args) == 1 && args[0] == "sources" {
		printSources(stdout)
		return 0
	}
	if len(args) == 0 || args[0] != "search" {
		fmt.Fprintln(stderr, "usage: pansou <search|recent|sources> [args]")
		return 1
	}
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sources := fs.String("source", "tg", "comma-separated sources (default: tg; available: tg,pan666)")
	channels := fs.String("channel", defaultChannels, "comma-separated Telegram channels")
	types := fs.String("type", "", "comma-separated link types")
	limit := fs.Int("limit", 20, "maximum results (0 = no limit)")
	timeout := fs.Duration("timeout", 8*time.Second, "overall timeout")
	parallel := fs.Int("parallel", 4, "maximum concurrent sources")
	format := fs.String("format", "text", "text, json, or ndjson")
	noRank := fs.Bool("no-rank", false, "do not rank results")
	noDedupe := fs.Bool("no-dedupe", false, "do not deduplicate results")
	noProgress := fs.Bool("no-progress", false, "do not show elapsed search time on stderr")
	verbose := fs.Bool("verbose", false, "show per-source errors instead of a summary")
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			fmt.Fprintln(stdout, "usage: pansou search <query> [flags]")
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
	}
	flagArgs, query, err := partitionSearchArgs(args[1:])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}
	if query == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "search requires exactly one query")
		return 1
	}
	q := core.Query{Text: query, Sources: split(*sources), Types: split(*types), Limit: *limit, Timeout: *timeout, NoRank: *noRank, NoDedupe: *noDedupe}
	all := availableSources(split(*channels))
	q.Sources = expandAll(q.Sources, all)
	if err := validateSources(q.Sources, all); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), q.Timeout)
	defer cancel()
	stopProgress := startProgress(stderr, *noProgress)
	result := app.Runner{Sources: all, Parallel: *parallel}.Run(ctx, q)
	stopProgress(result)
	if err := output.Write(stdout, *format, result); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	printWarnings(stderr, result.Warnings, *verbose)
	if len(result.Items) == 0 && len(result.Warnings) == 0 {
		fmt.Fprintln(stderr, "no results")
	}
	if len(result.Items) == 0 && len(result.Warnings) > 0 {
		return 2
	}
	return 0
}

func availableSources(channels []string) []app.Source {
	all := []app.Source{tg.Source{Channels: channels}}
	for _, source := range legacy.Sources() {
		all = append(all, source)
	}
	return all
}

func expandAll(selected []string, all []app.Source) []string {
	for _, name := range selected {
		if name == "all" {
			names := make([]string, 0, len(all))
			for _, source := range all {
				names = append(names, source.Name())
			}
			return names
		}
	}
	return selected
}

func startProgress(stderr io.Writer, disabled bool) func(core.Result) {
	if disabled {
		return func(core.Result) {}
	}
	started := time.Now()
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		defer close(finished)
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(stderr, "\rsearching… %s", time.Since(started).Round(time.Second))
			case <-done:
				return
			}
		}
	}()
	return func(result core.Result) {
		close(done)
		<-finished
		fmt.Fprintf(stderr, "\rsearch finished in %s · %d results\n", result.Elapsed.Round(time.Millisecond), len(result.Items))
	}
}

func printWarnings(stderr io.Writer, warnings []core.Warning, verbose bool) {
	if len(warnings) == 0 {
		return
	}
	timeouts := 0
	failed := 0
	for _, warning := range warnings {
		if strings.Contains(warning.Message, context.DeadlineExceeded.Error()) {
			timeouts++
		} else {
			failed++
		}
	}
	if !verbose {
		parts := make([]string, 0, 2)
		if failed > 0 {
			parts = append(parts, fmt.Sprintf("%d failed", failed))
		}
		if timeouts > 0 {
			parts = append(parts, fmt.Sprintf("%d timed out", timeouts))
		}
		fmt.Fprintf(stderr, "sources: %s (use --verbose for details)\n", strings.Join(parts, ", "))
		return
	}
	for _, warning := range warnings {
		if warning.Source != "" {
			fmt.Fprintf(stderr, "warning: %s: %s\n", warning.Source, warning.Message)
		} else {
			fmt.Fprintf(stderr, "warning: %s\n", warning.Message)
		}
	}
}

func runRecent(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("recent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 10, "maximum messages (0 = all messages on the public page)")
	timeout := fs.Duration("timeout", 8*time.Second, "request timeout")
	format := fs.String("format", "text", "text or json")
	flagArgs, channel, err := partitionArgs(args, map[string]bool{"--limit": true, "--timeout": true, "--format": true}, nil, "recent")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}
	if channel == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: pansou recent <channel> [--limit 10] [--format text|json]")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	messages, err := (tg.Source{}).Recent(ctx, channel, *limit)
	if err != nil {
		fmt.Fprintf(stderr, "recent: %v\n", err)
		return 2
	}
	if *format == "json" {
		if err := json.NewEncoder(stdout).Encode(messages); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if *format != "text" {
		fmt.Fprintln(stderr, "recent: format must be text or json")
		return 1
	}
	if len(messages) == 0 {
		fmt.Fprintln(stderr, "no recent messages")
		return 0
	}
	for i, message := range messages {
		fmt.Fprintf(stdout, "%d. %s  %s\n", i+1, message.PublishedAt.Format("2006-01-02 15:04"), message.Title)
		if message.Content != "" {
			fmt.Fprintf(stdout, "   %s\n", message.Content)
		}
		for _, link := range message.Links {
			fmt.Fprintf(stdout, "   [%s] %s\n", link.Type, link.URL)
		}
	}
	return 0
}

func printSources(w io.Writer) {
	fmt.Fprintln(w, "Default sources:")
	fmt.Fprintln(w, "  tg (enabled by default)")
	fmt.Fprintln(w, "    channels:")
	for _, channel := range split(defaultChannels) {
		fmt.Fprintf(w, "      - %s\n", channel)
	}
	legacySources := legacy.Sources()
	fmt.Fprintf(w, "  %d legacy plugins (use --source all to search every plugin):\n", len(legacySources))
	for _, source := range legacySources {
		fmt.Fprintf(w, "      - %s\n", source.Name())
	}
	fmt.Fprintln(w, "\nOverride Telegram channels with: pansou search <query> --channel channel_a,channel_b")
}
func split(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
func validateSources(selected []string, all []app.Source) error {
	known := map[string]bool{}
	available := make([]string, 0, len(all))
	for _, source := range all {
		known[source.Name()] = true
		available = append(available, source.Name())
	}
	sort.Strings(available)
	for _, name := range selected {
		if !known[name] {
			return fmt.Errorf("unknown source %q (run %q to list %d available sources)", name, "pansou sources", len(available))
		}
	}
	return nil
}

// partitionSearchArgs lets users put the query before or after flags, while
// retaining the standard library flag package for the actual flag parsing.
func partitionSearchArgs(args []string) ([]string, string, error) {
	return partitionArgs(args, map[string]bool{"--source": true, "--channel": true, "--type": true, "--limit": true, "--timeout": true, "--parallel": true, "--format": true}, map[string]bool{"--no-rank": true, "--no-dedupe": true, "--no-progress": true, "--verbose": true}, "search")
}

func partitionArgs(args []string, valueFlags, boolFlags map[string]bool, command string) ([]string, string, error) {
	var flags []string
	argument := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			name := strings.SplitN(arg, "=", 2)[0]
			flags = append(flags, arg)
			if valueFlags[name] && !strings.Contains(arg, "=") {
				if i+1 >= len(args) {
					return nil, "", fmt.Errorf("flag %s requires a value", name)
				}
				i++
				flags = append(flags, args[i])
			} else if !valueFlags[name] && !boolFlags[name] {
				return nil, "", fmt.Errorf("unknown flag %s", name)
			}
			continue
		}
		if argument != "" {
			return nil, "", fmt.Errorf("%s accepts exactly one argument", command)
		}
		argument = arg
	}
	return flags, argument, nil
}

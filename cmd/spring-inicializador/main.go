package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/extract"
	"github.com/miferdev/spring-inicializador/internal/options"
	"github.com/miferdev/spring-inicializador/internal/render"
	"github.com/miferdev/spring-inicializador/internal/tui"
)

// version is overridden at build time with:
//
//	go build -ldflags "-X main.version=<version>" ./cmd/spring-inicializador
var version = "dev"

const (
	exitOK          = 0
	exitFailure     = 1
	exitUsage       = 2
	exitInterrupted = 130

	// defaultServer is the public Initializr. --server replaces it for
	// self-hosted instances, proxies and air-gapped networks.
	defaultServer = "https://start.spring.io"

	// requestTimeout bounds both the metadata fetch and the generation call so
	// a stalled upstream fails instead of hanging a pipeline job.
	requestTimeout = 60 * time.Second
)

const usageText = `spring-inicializador - generate a Spring Boot project from the command line

Usage:
  spring-inicializador [flags]

Flags:
  --deps IDs         Comma-separated dependency ids, e.g. web,data-jpa
  --output DIR       Directory that receives the generated project (default ".")
  --force            Overwrite an existing output directory
  --group-id ID      Project group id (default: from server metadata)
  --artifact-id ID   Project artifact id (default: from server metadata)
  --server URL       Initializr base URL for metadata and generation
                     (default "https://start.spring.io")
  --yes              Run without prompting, taking defaults
  --version          Print the version and exit
  --help, -h         Print this help and exit

With no project flag, no --yes and a terminal on stdin, it asks for the
project options interactively. Anything else stays non-interactive.

The archive is extracted into <output>/<artifact-id>.

Exit codes: 0 success, 1 server or filesystem failure, 2 usage error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// commandLine is the parsed command line. One struct keeps every flag, its
// default and its documentation together as US2.3 adds one per form field.
type commandLine struct {
	deps       string
	output     string
	force      bool
	groupID    string
	artifactID string
	server     string
	yes        bool
	version    bool
	help       bool
}

// newFlagSet registers the documented flags. It is separate from run so tests
// can check which flags answer the form without starting the pipeline.
func newFlagSet(output io.Writer) (*flag.FlagSet, *commandLine) {
	flags := flag.NewFlagSet("spring-inicializador", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { fmt.Fprint(flags.Output(), usageText) }

	cmd := &commandLine{}
	flags.StringVar(&cmd.deps, "deps", "", "comma-separated dependency ids, e.g. web,data-jpa")
	flags.StringVar(&cmd.output, "output", ".", "directory that receives the generated project")
	flags.BoolVar(&cmd.force, "force", false, "overwrite an existing output directory")
	flags.StringVar(&cmd.groupID, "group-id", "", "project group id (default: from server metadata)")
	flags.StringVar(&cmd.artifactID, "artifact-id", "", "project artifact id (default: from server metadata)")
	flags.StringVar(&cmd.server, "server", defaultServer, "Initializr base URL for metadata and generation")
	flags.BoolVar(&cmd.yes, "yes", false, "run without prompting, taking defaults")
	flags.BoolVar(&cmd.version, "version", false, "print the version and exit")
	flags.BoolVar(&cmd.help, "help", false, "print this help and exit")
	flags.BoolVar(&cmd.help, "h", false, "print this help and exit (shorthand)")
	return flags, cmd
}

// formFlags are the flags whose value already answers a row of the form.
// Passing one means the user is scripting the run, so the form must not open
// and ask for the rest.
var formFlags = []string{"group-id", "artifact-id", "deps"}

// passedFlags returns the names of the flags the user actually set, so a
// default value is never mistaken for a choice.
func passedFlags(flags *flag.FlagSet) map[string]bool {
	passed := make(map[string]bool)
	flags.Visit(func(flag *flag.Flag) { passed[flag.Name] = true })
	return passed
}

// mayPrompt reports whether the flag half of the gate allows the form: --yes is
// off and no flag answered a field already. The caller still has to check that
// stdin is a terminal, because a pipe must never wait for a keypress.
func mayPrompt(passed map[string]bool, yes bool) bool {
	if yes {
		return false
	}
	for _, name := range formFlags {
		if passed[name] {
			return false
		}
	}
	return true
}

// isTerminal reports whether f is a character device, the standard-library
// signal that a person can answer a prompt. x/term would be a dependency for a
// one-line check.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// run is the whole program without process-level side effects: it reads stdin
// only to run the interactive form, writes ASCII-only progress to stdout,
// errors to stderr, and returns the exit code. It exists so the pipeline can be
// tested end to end in-process.
func run(args []string, stdout, stderr io.Writer) int {
	flags, cmd := newFlagSet(stderr)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usageText)
			return exitOK
		}
		return exitUsage
	}

	if cmd.help {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	if cmd.version {
		fmt.Fprintf(stdout, "spring-inicializador %s\n", version)
		return exitOK
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "spring-inicializador: unexpected argument %q\n", flags.Arg(0))
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}

	// One base URL for both endpoints. A malformed --server is a usage error
	// and fails before any request is built.
	baseURL, err := catalog.NormalizeBaseURL(cmd.server)
	if err != nil {
		fmt.Fprintf(stderr, "spring-inicializador: %v\n", err)
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := &http.Client{Timeout: requestTimeout}

	c, err := catalog.Fetch(ctx, client, baseURL)
	if err != nil {
		return reportError(stderr, err)
	}

	opts := resolve(c, cmd.groupID, cmd.artifactID, cmd.deps)

	// The form is the default interaction, but only when a person is there to
	// answer it: no generation flag already answered a field, --yes is off and
	// stdin is a terminal. A pipe or a redirection keeps the old, fully
	// scriptable path unchanged.
	if mayPrompt(passedFlags(flags), cmd.yes) && isTerminal(os.Stdin) {
		chosen, ok, err := tui.RunForm(c, os.Stdin, stdout)
		if err != nil {
			return reportError(stderr, err)
		}
		if !ok {
			// Aborted with esc or ctrl+c: nothing to do, nothing written.
			return exitOK
		}
		opts = chosen
	}

	archive, err := render.Render(ctx, client, baseURL, opts)
	if err != nil {
		return reportError(stderr, err)
	}

	overwrote, err := extract.Extract(archive, cmd.output, cmd.force)
	if err != nil {
		return reportError(stderr, err)
	}

	// The server names the archive's top-level directory after baseDir, which
	// this tool always sets to the artifact id.
	destination := filepath.Join(cmd.output, opts.ArtifactID)
	if overwrote {
		fmt.Fprintf(stdout, "Overwrote %s\n", destination)
	} else {
		fmt.Fprintf(stdout, "Created %s\n", destination)
	}
	fmt.Fprintf(stdout, "Next: cd %s && %s\n", destination, runCommand(opts))
	return exitOK
}

// resolve starts from the metadata defaults and applies the identifier flags.
// When either identifier changes, the name and package name that were derived
// from the defaults follow it, so the project stays internally consistent.
func resolve(c *catalog.Catalog, groupID, artifactID, deps string) options.Options {
	defaults := options.Defaults(c)
	opts := defaults

	if groupID != "" {
		opts.GroupID = strings.TrimSpace(groupID)
	}
	if artifactID != "" {
		opts.ArtifactID = strings.TrimSpace(artifactID)
		if opts.Name == defaults.ArtifactID {
			opts.Name = opts.ArtifactID
		}
	}
	if groupID != "" || artifactID != "" {
		opts.PackageName = options.DerivePackageName(opts.GroupID, opts.ArtifactID)
	}

	opts.Deps = splitDeps(deps)
	return opts
}

// splitDeps turns the --deps value into ids, keeping the user's order and
// dropping empty segments. It does not validate ids: the server rejects an
// unknown dependency with a message worth printing verbatim.
func splitDeps(list string) []string {
	if strings.TrimSpace(list) == "" {
		return nil
	}

	parts := strings.Split(list, ",")
	deps := make([]string, 0, len(parts))
	for _, part := range parts {
		if id := strings.TrimSpace(part); id != "" {
			deps = append(deps, id)
		}
	}
	return deps
}

func runCommand(opts options.Options) string {
	switch opts.Build {
	case options.BuildGradle, options.BuildGradleKotlin:
		return "./gradlew bootRun"
	default:
		return "./mvnw spring-boot:run"
	}
}

// reportError prints one prefixed line to stderr and maps the failure to its
// exit code. An interrupted run is reported as 130 so shells and CI can tell it
// apart from a real failure.
func reportError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "spring-inicializador: %v\n", err)
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "spring-inicializador: interrupted")
		return exitInterrupted
	}
	return exitFailure
}

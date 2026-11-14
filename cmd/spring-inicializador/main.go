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
  --group-id ID      Project group id (default: from server metadata)
  --artifact-id ID   Project artifact id (default: from server metadata)
  --server URL       Initializr base URL for metadata and generation
                     (default "https://start.spring.io")
  --yes              Run without prompting, taking defaults
  --version          Print the version and exit
  --help, -h         Print this help and exit

The archive is extracted into <output>/<artifact-id>.

Exit codes: 0 success, 1 server or filesystem failure, 2 usage error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program without process-level side effects: it never reads
// stdin, writes ASCII-only progress to stdout, errors to stderr, and returns
// the exit code. It exists so the pipeline can be tested end to end in-process.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("spring-inicializador", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(flags.Output(), usageText) }

	deps := flags.String("deps", "", "comma-separated dependency ids, e.g. web,data-jpa")
	output := flags.String("output", ".", "directory that receives the generated project")
	groupID := flags.String("group-id", "", "project group id (default: from server metadata)")
	artifactID := flags.String("artifact-id", "", "project artifact id (default: from server metadata)")
	server := flags.String("server", defaultServer, "Initializr base URL for metadata and generation")
	// Accepted for CI parity. This story has no prompts at all, so there is
	// nothing for it to suppress yet.
	flags.Bool("yes", false, "run without prompting, taking defaults")
	showVersion := flags.Bool("version", false, "print the version and exit")
	showHelp := flags.Bool("help", false, "print this help and exit")
	flags.BoolVar(showHelp, "h", false, "print this help and exit (shorthand)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usageText)
			return exitOK
		}
		return exitUsage
	}

	if *showHelp {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	if *showVersion {
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
	baseURL, err := catalog.NormalizeBaseURL(*server)
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

	opts := resolve(c, *groupID, *artifactID, *deps)

	archive, err := render.Render(ctx, client, baseURL, opts)
	if err != nil {
		return reportError(stderr, err)
	}

	if err := extract.Extract(archive, *output); err != nil {
		return reportError(stderr, err)
	}

	// The server names the archive's top-level directory after baseDir, which
	// this tool always sets to the artifact id.
	destination := filepath.Join(*output, opts.ArtifactID)
	fmt.Fprintf(stdout, "Created %s\n", destination)
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

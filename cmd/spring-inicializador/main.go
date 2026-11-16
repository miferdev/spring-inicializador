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
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
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
  --name NAME        Project name (default: artifact id)
  --package-name ID  Package name (default: derived from group and artifact)
  --build TYPE       Build tool: maven, gradle, gradle-kotlin (default "maven")
  --language LANG    Language: java, kotlin, groovy (default: from server metadata)
  --packaging PKG    Packaging: jar, war (default: from server metadata)
  --java VERSION     Java version, e.g. 21 (default: from server metadata)
  --boot-version VER Spring Boot version, e.g. 4.1.2.RELEASE (default: server default)
  --server URL       Initializr base URL for metadata and generation
                     (default "https://start.spring.io")
  --dry-run          Print the resolved options and exit without generating
  --yes              Run without prompting, taking defaults
  --fancy            Use Unicode borders and glyphs instead of ASCII
  --no-color         Strip every style; NO_COLOR in the environment does the same
  --version          Print the version and exit
  --help, -h         Print this help and exit

On a terminal, without --yes, it asks for every project option a flag did not
provide. Anything else stays non-interactive.

The archive is extracted into <output>/<artifact-id>.

Exit codes: 0 success, 1 server or filesystem failure, 2 usage error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// commandLine is the parsed command line. One struct keeps every flag, its
// default and its documentation together: each project flag answers exactly the
// form row of the same name.
type commandLine struct {
	deps        string
	output      string
	force       bool
	groupID     string
	artifactID  string
	name        string
	packageName string
	build       string
	language    string
	packaging   string
	javaVersion string
	bootVersion string
	server      string
	dryRun      bool
	yes         bool
	fancy       bool
	noColor     bool
	version     bool
	help        bool
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
	flags.StringVar(&cmd.name, "name", "", "project name (default: artifact id)")
	flags.StringVar(&cmd.packageName, "package-name", "", "package name (default: derived from group and artifact)")
	flags.StringVar(&cmd.build, "build", "", "build tool: maven, gradle, gradle-kotlin (default: maven)")
	flags.StringVar(&cmd.language, "language", "", "language: java, kotlin, groovy (default: from server metadata)")
	flags.StringVar(&cmd.packaging, "packaging", "", "packaging: jar, war (default: from server metadata)")
	flags.StringVar(&cmd.javaVersion, "java", "", "Java version, e.g. 21 (default: from server metadata)")
	flags.StringVar(&cmd.bootVersion, "boot-version", "", "Spring Boot version, e.g. 4.1.2.RELEASE (default: server default)")
	flags.StringVar(&cmd.server, "server", defaultServer, "Initializr base URL for metadata and generation")
	flags.BoolVar(&cmd.dryRun, "dry-run", false, "print the resolved options and exit without generating")
	flags.BoolVar(&cmd.yes, "yes", false, "run without prompting, taking defaults")
	flags.BoolVar(&cmd.fancy, "fancy", false, "use Unicode borders and glyphs instead of ASCII")
	flags.BoolVar(&cmd.noColor, "no-color", false, "strip every style; NO_COLOR in the environment does the same")
	flags.BoolVar(&cmd.version, "version", false, "print the version and exit")
	flags.BoolVar(&cmd.help, "help", false, "print this help and exit")
	flags.BoolVar(&cmd.help, "h", false, "print this help and exit (shorthand)")
	return flags, cmd
}

// passedFlags returns the names of the flags the user actually set, so a
// default value is never mistaken for a choice.
func passedFlags(flags *flag.FlagSet) map[string]bool {
	passed := make(map[string]bool)
	flags.Visit(func(flag *flag.Flag) { passed[flag.Name] = true })
	return passed
}

// providedByFlags maps each project field the command line answered to the
// value it set. The form leaves those rows out and keeps the values. --deps is
// absent on purpose: the dependency pane belongs to VS3, not to this form.
func providedByFlags(cmd *commandLine, passed map[string]bool) tui.Values {
	provided := tui.Values{}
	if passed["group-id"] {
		provided[tui.KeyGroupID] = strings.TrimSpace(cmd.groupID)
	}
	if passed["artifact-id"] {
		provided[tui.KeyArtifactID] = strings.TrimSpace(cmd.artifactID)
	}
	if passed["name"] {
		provided[tui.KeyName] = strings.TrimSpace(cmd.name)
	}
	if passed["package-name"] {
		provided[tui.KeyPackageName] = strings.TrimSpace(cmd.packageName)
	}
	if passed["build"] {
		provided[tui.KeyBuild] = cmd.build
	}
	if passed["language"] {
		provided[tui.KeyLanguage] = cmd.language
	}
	if passed["packaging"] {
		provided[tui.KeyPackaging] = cmd.packaging
	}
	if passed["java"] {
		provided[tui.KeyJavaVersion] = cmd.javaVersion
	}
	if passed["boot-version"] {
		provided[tui.KeyBootVersion] = cmd.bootVersion
	}
	return provided
}

// validateVocabulary rejects a value from this tool's own vocabulary before any
// request: a typo should not cost a round trip, and the server would only echo
// the same refusal back.
func validateVocabulary(cmd *commandLine, passed map[string]bool) error {
	checks := []struct {
		flag     string
		value    string
		accepted []string
	}{
		{"build", cmd.build, options.Builds()},
		{"language", cmd.language, options.Languages()},
		{"packaging", cmd.packaging, options.Packagings()},
	}

	for _, check := range checks {
		if !passed[check.flag] {
			continue
		}
		if !slices.Contains(check.accepted, check.value) {
			return fmt.Errorf("--%s %q is not valid; accepted: %s",
				check.flag, check.value, strings.Join(check.accepted, ", "))
		}
	}
	return nil
}

// validatePublished rejects a value only the server can vouch for. It runs on
// the fetched catalog, before generation, so the message names the versions that
// actually exist instead of relaying a 400 from the server.
func validatePublished(c *catalog.Catalog, cmd *commandLine, passed map[string]bool) error {
	checks := []struct {
		flag      string
		value     string
		published catalog.Select
	}{
		{"java", cmd.javaVersion, c.JavaVersion},
		{"boot-version", cmd.bootVersion, c.BootVersion},
	}

	for _, check := range checks {
		if !passed[check.flag] {
			continue
		}
		if !publishedChoice(check.published, check.value) {
			return fmt.Errorf("--%s %q is not published by the server; published: %s",
				check.flag, check.value, choiceIDs(check.published))
		}
	}
	return nil
}

func publishedChoice(sel catalog.Select, id string) bool {
	for _, value := range sel.Values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func choiceIDs(sel catalog.Select) string {
	if len(sel.Values) == 0 {
		return "none"
	}

	ids := make([]string, 0, len(sel.Values))
	for _, value := range sel.Values {
		ids = append(ids, value.ID)
	}
	return strings.Join(ids, ", ")
}

// interactiveStdin reports whether a person can answer a prompt. A character
// device is not enough to answer that question: /dev/null is one, and treating
// it as a terminal made any scripted run that redirected stdin without --yes
// try to open the form and fail. isatty is the only honest check.
//
// It is a variable so a test can prove a run opens (or refuses to open) the
// form without needing a terminal of its own.
var interactiveStdin = func(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// appearance is the theming a run asked for. NO_COLOR is honoured by presence, not
// by value — even an empty setting disables color, which is what the convention
// says — and --fancy is only ever opt-in.
func appearance(fancy, noColor bool) tui.Settings {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		noColor = true
	}
	return tui.Settings{Fancy: fancy, NoColor: noColor}
}

// runForm is the interactive wizard. It is a variable so a test can prove a run
// does not open it, without needing a terminal of its own.
var runForm = tui.Run

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

	passed := passedFlags(flags)

	// The tool's own vocabulary is checked before any request: a typo must not
	// cost a round trip.
	if err := validateVocabulary(cmd, passed); err != nil {
		fmt.Fprintf(stderr, "spring-inicializador: %v\n", err)
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

	// The run report: one line per pipeline step, printed as it finishes so a slow
	// step reads as work rather than a hang (US6.2). A dry run prints the request
	// instead, so it gets no report at all.
	report := stepReport{out: stdout}

	c, err := catalog.Fetch(ctx, client, baseURL)
	if err != nil {
		if !cmd.dryRun {
			report.fail(stepCatalog)
		}
		return reportError(stderr, err)
	}
	if !cmd.dryRun {
		report.ok(stepCatalog, count(len(c.Dependencies), "dependency", "dependencies"))
	}

	// --java and --boot-version are the server's vocabulary: now that the catalog
	// is here, check them against what it published, before generation.
	if err := validatePublished(c, cmd, passed); err != nil {
		fmt.Fprintf(stderr, "spring-inicializador: %v\n", err)
		return exitUsage
	}

	opts := resolve(c, cmd, passed)
	provided := providedByFlags(cmd, passed)

	// On a terminal the form asks for exactly what the flags did not answer.
	// --yes and a redirected stdin both keep a run scripted, and a run whose
	// flags answered every row has nothing left to ask.
	if !cmd.yes && interactiveStdin(os.Stdin) && tui.Asks(c, provided) {
		// --deps answers the dependency screen the way the project flags answer
		// its rows: the ids arrive already checked, and the screen decides.
		// --output is where the review says the project will land; it is not part of
		// the request the server receives (US6.1).
		chosen, ok, err := runForm(c, provided, splitDeps(cmd.deps), appearance(cmd.fancy, cmd.noColor), cmd.output, os.Stdin, stdout)
		if err != nil {
			return reportError(stderr, err)
		}
		if !ok {
			// Aborted with ctrl+c: nothing to do, nothing written.
			return exitOK
		}
		opts = chosen
	}

	// --dry-run resolves everything a real run would and then prints it instead
	// of generating: no /starter.zip request, no extraction, no output directory.
	if cmd.dryRun {
		writeDryRun(stdout, opts)
		return exitOK
	}

	archive, err := render.Render(ctx, client, baseURL, opts)
	if err != nil {
		report.fail(stepGenerating)
		return reportError(stderr, err)
	}
	report.ok(stepGenerating, size(len(archive)))

	result, err := extract.Extract(archive, cmd.output, cmd.force)
	if err != nil {
		report.fail(stepExtracting)
		return reportError(stderr, err)
	}
	report.ok(stepExtracting, count(result.Files, "file", "files"))
	// The permissions step has no pass of its own: the extraction applies the
	// archive's bits, so the step reports exactly what it made executable.
	report.ok(stepPermissions, executables(result.Executable, opts.ArtifactID))

	// The server names the archive's top-level directory after baseDir, which
	// this tool always sets to the artifact id.
	destination := filepath.Join(cmd.output, opts.ArtifactID)
	if result.Replaced {
		fmt.Fprintf(stdout, "Overwrote %s\n", destination)
	} else {
		fmt.Fprintf(stdout, "Created %s\n", destination)
	}
	fmt.Fprintf(stdout, "Next: cd %s && %s\n", destination, runCommand(opts))
	return exitOK
}

// resolve starts from the metadata defaults and applies every value the command
// line gave, so a scripted run asks the server for exactly what was typed.
func resolve(c *catalog.Catalog, cmd *commandLine, passed map[string]bool) options.Options {
	defaults := options.Defaults(c)
	opts := defaults

	for key, value := range providedByFlags(cmd, passed) {
		switch key {
		case tui.KeyGroupID:
			opts.GroupID = value
		case tui.KeyArtifactID:
			opts.ArtifactID = value
		case tui.KeyName:
			opts.Name = value
		case tui.KeyPackageName:
			opts.PackageName = value
		case tui.KeyBuild:
			opts.Build = value
		case tui.KeyLanguage:
			opts.Language = value
		case tui.KeyPackaging:
			opts.Packaging = value
		case tui.KeyJavaVersion:
			opts.JavaVersion = value
		case tui.KeyBootVersion:
			opts.BootVersion = value
		}
	}

	// When the artifact id moves and nobody named the project, the name and the
	// package follow it, so the request stays internally consistent.
	if passed["artifact-id"] && !passed["name"] && opts.Name == defaults.ArtifactID {
		opts.Name = opts.ArtifactID
	}
	if !passed["package-name"] && (passed["group-id"] || passed["artifact-id"]) {
		opts.PackageName = options.DerivePackageName(opts.GroupID, opts.ArtifactID)
	}

	opts.Deps = splitDeps(cmd.deps)
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

// dryRunField is one line of the dry run report.
type dryRunField struct {
	key   string
	value string
}

// dryRunReport is the only place that decides what a dry run prints and in which
// order: the project fields the form fills, then the dependencies. The order and
// the keys are fixed on purpose, because the output is meant to be diffed.
func dryRunReport(opts options.Options) []dryRunField {
	return []dryRunField{
		{"group-id", opts.GroupID},
		{"artifact-id", opts.ArtifactID},
		{"name", opts.Name},
		{"package-name", opts.PackageName},
		{"build", opts.Build},
		{"language", opts.Language},
		{"packaging", opts.Packaging},
		{"java", opts.JavaVersion},
		{"boot-version", opts.BootVersion},
		{"deps", strings.Join(opts.Deps, ",")},
	}
}

// writeDryRun prints the resolved request, one "key=value" line per field. No
// timestamps, colors, paths or width-dependent alignment: two identical
// invocations must produce identical bytes.
func writeDryRun(stdout io.Writer, opts options.Options) {
	for _, field := range dryRunReport(opts) {
		fmt.Fprintf(stdout, "%s=%s\n", field.key, field.value)
	}
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

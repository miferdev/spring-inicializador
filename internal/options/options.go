// Package options holds the single project description that both the command
// line and (later) the interactive form produce. Options is the only value that
// reaches render, so the two frontends cannot drift into parallel code paths.
//
// There is deliberately no validation here yet: the server owns value
// validation and its message is printed verbatim (US1.4).
package options

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// Build identifiers accepted on the command line and in the form.
const (
	BuildMaven        = "maven"
	BuildGradle       = "gradle"
	BuildGradleKotlin = "gradle-kotlin"
)

// buildTypes is the only place that knows how a build name becomes a metadata
// `type` id and back. BuildType and BuildName read the same table, so the flag
// and the form cannot disagree about what a build is called.
var buildTypes = map[string]string{
	BuildMaven:        "maven-project",
	BuildGradle:       "gradle-project",
	BuildGradleKotlin: "gradle-project-kotlin",
}

// Options is one project request. Every field is a plain string because every
// transport form (flags today, form fields later) is textual too.
type Options struct {
	GroupID     string
	ArtifactID  string
	Name        string
	Description string
	PackageName string

	Build    string // maven | gradle | gradle-kotlin
	Language string // java | kotlin | groovy

	Packaging   string
	JavaVersion string
	BootVersion string // empty means "let the server pick"

	Deps []string // dependency ids, in the order the user gave them
}

// Defaults resolves every default the metadata document provides. Decision D3
// overrides exactly one of them: the build is Maven, not the metadata's
// `gradle-project`, so the flag and the UI agree.
func Defaults(c *catalog.Catalog) Options {
	opts := Options{Build: BuildMaven}
	if c == nil {
		return opts
	}

	opts.GroupID = strings.TrimSpace(c.Text["groupId"])
	opts.ArtifactID = strings.TrimSpace(c.Text["artifactId"])
	opts.Language = c.Language.Default
	opts.Packaging = c.Packaging.Default
	opts.JavaVersion = c.JavaVersion.Default
	opts.BootVersion = c.BootVersion.Default
	opts.Description = strings.TrimSpace(c.Text["description"])

	// name and packageName have no `default` in the metadata, so both are
	// derived from the identifiers the server did provide.
	opts.Name = strings.TrimSpace(c.Text["name"])
	if opts.Name == "" {
		opts.Name = opts.ArtifactID
	}

	opts.PackageName = strings.TrimSpace(c.Text["packageName"])
	if opts.PackageName == "" {
		opts.PackageName = DerivePackageName(opts.GroupID, opts.ArtifactID)
	}

	return opts
}

// DerivePackageName builds the conventional package name from the project
// identifiers, the same way the server does when no packageName is supplied:
// group id first, then artifact id, joined with dots, with characters that are
// illegal in a Java package removed. It is used when the metadata carries no
// packageName default and when the identifiers are overridden on the command
// line.
func DerivePackageName(groupID, artifactID string) string {
	parts := make([]string, 0, 2)
	for _, identifier := range []string{groupID, artifactID} {
		if segment := packageSegment(identifier); segment != "" {
			parts = append(parts, segment)
		}
	}
	return strings.Join(parts, ".")
}

func packageSegment(identifier string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(identifier) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Form returns the exact POST body for POST /starter.zip. bootVersion is only
// sent when it is set, and dependencies only when there are any, so the server
// applies its own defaults for anything the user did not choose.
func (o Options) Form() url.Values {
	form := url.Values{}
	form.Set("type", o.BuildType())
	form.Set("language", o.Language)
	if o.BootVersion != "" {
		form.Set("bootVersion", o.BootVersion)
	}
	form.Set("baseDir", o.ArtifactID)
	form.Set("groupId", o.GroupID)
	form.Set("artifactId", o.ArtifactID)
	form.Set("name", o.Name)
	form.Set("description", o.Description)
	form.Set("packageName", o.PackageName)
	form.Set("packaging", o.Packaging)
	form.Set("javaVersion", o.JavaVersion)
	if len(o.Deps) > 0 {
		form.Set("dependencies", strings.Join(o.Deps, ","))
	}
	return form
}

// BuildType maps the three user-facing build names to the metadata `type`
// values. An unknown build yields an empty type on purpose: the server rejects
// it with its own message instead of the client inventing a value.
func (o Options) BuildType() string {
	return buildTypes[o.Build]
}

// BuildName is the inverse of BuildType: it turns a metadata `type` id into the
// build name the flag and the form use, and reports false when the id is not a
// project archive. The server also publishes `maven-build` and `gradle-build`,
// which return a single file instead of a project; they have no build name
// here on purpose, so no frontend can offer them as an archive.
func BuildName(typeID string) (string, bool) {
	for build, id := range buildTypes {
		if id == typeID {
			return build, true
		}
	}
	return "", false
}

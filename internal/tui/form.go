// Package tui is the interactive frontend: one form over the metadata catalog
// that produces the same options.Options the flags produce. It never fetches,
// never renders an archive and never touches the filesystem, so the two
// frontends cannot drift into parallel code paths (see AGENTS.md).
package tui

import (
	"fmt"
	"slices"

	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// Key identifies one row of the project form. The set is closed: every key
// maps to exactly one field of formState.
type Key string

const (
	KeyGroupID     Key = "groupId"
	KeyArtifactID  Key = "artifactId"
	KeyName        Key = "name"
	KeyPackageName Key = "packageName"
	KeyBuild       Key = "build"
	KeyLanguage    Key = "language"
	KeyPackaging   Key = "packaging"
	KeyJavaVersion Key = "javaVersion"
	KeyBootVersion Key = "bootVersion"
)

// Values are the project values the command line already answered, keyed by the
// row they replace. A key present here is not asked for: the form leaves that
// row out and the value stays exactly as given.
type Values map[Key]string

// fieldKind says which widget renders a row.
type fieldKind int

const (
	fieldText fieldKind = iota
	fieldSelect
	// fieldNote replaces a select whose metadata key is missing: an empty
	// picker offers nothing to pick, so the row explains the gap instead.
	fieldNote
)

// choice is one select option. value is what lands in options.Options, label is
// what the user reads; they differ wherever the server id is not the user-facing
// name, as with a version id whose display name drops its qualifier.
type choice struct {
	value string
	label string
}

// field is the pure description of one row of the form. It is built from the
// catalog without a terminal, so tests can assert what the form offers and
// newForm is the only place that turns it into widgets.
type field struct {
	key     Key
	label   string
	kind    fieldKind
	initial string
	choices []choice
	note    string
}

// formState is the value of every row of the form. The widgets edit it in
// place and optionsFromState is the only consumer.
type formState struct {
	GroupID     string
	ArtifactID  string
	Name        string
	PackageName string
	Build       string
	Language    string
	Packaging   string
	JavaVersion string
	BootVersion string

	// The package field follows group and artifact until a human edits it.
	// packageAuto says whether it still does, and derivedPackage is the last
	// value derivation wrote: comparing the field against it is how an edit is
	// detected, since huh emits no per-field change event.
	packageAuto    bool
	derivedPackage string
}

// autoMarker is the visible sign that the package field is derived rather than
// typed. It is a field description, never part of the value, so copying the
// field yields a package name the server accepts.
const autoMarker = "(auto)"

// section is one page of the project form: a named slice of the rows that belong
// together. huh renders one group at a time, so a section is what the user reads
// while the other sections wait off screen (US7.1).
type section struct {
	name   string
	fields []field
}

// sectionKeys lists the form's pages in display order. A row belongs to exactly
// one page, which is what keeps the flat field list and the pages in step.
var sectionKeys = []struct {
	name string
	keys []Key
}{
	{name: "Identity", keys: []Key{KeyGroupID, KeyArtifactID, KeyName, KeyPackageName}},
	{name: "Build", keys: []Key{KeyBuild, KeyLanguage, KeyPackaging}},
	{name: "Versions", keys: []Key{KeyJavaVersion, KeyBootVersion}},
}

// splitSections partitions the described rows into the pages the form shows,
// keeping the page order and the row order inside a page. A page whose rows the
// flags all answered keeps its place and simply has no rows (US7.2 decides whether
// it keeps its number).
func splitSections(fields []field) []section {
	sections := make([]section, len(sectionKeys))
	for i, known := range sectionKeys {
		sections[i] = section{name: known.name}
	}

	for _, f := range fields {
		for i, known := range sectionKeys {
			if slices.Contains(known.keys, f.key) {
				sections[i].fields = append(sections[i].fields, f)
				break
			}
		}
	}
	return sections
}

// title is the line the frame shows above a section: its name and its place among
// every section, so nothing has to be counted by eye. The total is the number of
// sections, not of the ones with rows, until US7.2 renumbers the sections it hides
// (US7.1).
func (s section) title(index, total int) string {
	return fmt.Sprintf("%s %d/%d", s.name, index, total)
}

// projectForm is the project form: the field descriptions, the state they
// edit, and the huh form that renders them.
type projectForm struct {
	// fields is every row the form asks, in display order; sections is the same
	// list partitioned into the pages that are shown one at a time.
	sections []section
	fields   []field
	state    formState
	form     *huh.Form

	// packageInput is the package row. The update loop needs it to show a
	// freshly derived value and the (auto) marker, because huh's input keeps a
	// second copy of the text.
	packageInput *huh.Input

	// firstKey is the key of the first row the form asks. "Back" has nowhere to
	// go from there, which is how esc tells abort apart from a step back.
	firstKey Key

	// theme is the appearance the flags chose. The form draws its own border, so
	// it is part of the widgets, not of the view around them (US5.4).
	theme Theme
}

// newProjectForm builds the form from the catalog and starts every row on its
// default, then applies the values the command line already answered: those rows
// are left out, and their values seed the state so the resolved Options carry
// them. It is the only constructor: tests assert on the fields it produces and
// Run renders the same form.
func newProjectForm(cat *catalog.Catalog, provided Values, theme Theme) *projectForm {
	fields := buildFields(cat, provided)
	form := &projectForm{
		sections: splitSections(fields),
		fields:   fields,
		theme:    theme,
	}
	form.firstKey = firstAskedKey(form.fields)

	values := form.state.pointers()
	for _, f := range form.fields {
		if value, ok := values[f.key]; ok {
			*value = f.initial
		}
	}
	for key, value := range provided {
		if ptr, ok := values[key]; ok {
			*ptr = value
		}
	}

	if _, answered := provided[KeyPackageName]; answered {
		// The human decided the package name: derivation stays off and nothing
		// later may replace it.
		form.state.packageAuto = false
	} else {
		// The seeded package name counts as the first derived value: until a
		// human types something else, the field follows group and artifact.
		form.state.armPackage()

		// Identifiers that came from flags are already typed as far as the form
		// is concerned, so the first frame shows the name they derive.
		_, group := provided[KeyGroupID]
		_, artifact := provided[KeyArtifactID]
		if group || artifact {
			form.state.followPackage()
		}
	}

	form.form, form.packageInput = newForm(form.sections, values, theme)
	if form.packageInput != nil {
		form.packageInput.Description(form.state.packageMarker())
	}
	return form
}

// reopen rebuilds the widgets over the state the form left behind. A huh form
// that completed renders nothing and ignores every message, so a screen that
// comes back needs a live view over the same values; the values are the state,
// and nothing here copies them.
func (f *projectForm) reopen() {
	values := f.state.pointers()
	f.form, f.packageInput = newForm(f.sections, values, f.theme)
	if f.packageInput != nil {
		f.packageInput.Description(f.state.packageMarker())
	}
}

// asks reports whether the form has a question left. A row the flags answered is
// gone, and a metadata gap is a note rather than a question.
func (f *projectForm) asks() bool {
	return asksFields(f.fields)
}

// Asks reports whether the form still has a question for the values the command
// line already answered. When it returns false there is nothing to render, and a
// run must not wait for a keypress.
func Asks(cat *catalog.Catalog, provided Values) bool {
	return asksFields(buildFields(cat, provided))
}

func asksFields(fields []field) bool {
	for _, field := range fields {
		if field.kind != fieldNote {
			return true
		}
	}
	return false
}

// firstAskedKey is the key of the first row the form actually asks. A note is
// skipped, so the focus never rests on one.
func firstAskedKey(fields []field) Key {
	for _, field := range fields {
		if field.kind != fieldNote {
			return field.key
		}
	}
	return ""
}

// buildFields describes the project form in display order, skipping the rows the
// command line already answered. Every offering comes from the catalog, so a
// server that stops publishing a value stops offering it: no option value is
// written into source code.
func buildFields(cat *catalog.Catalog, provided Values) []field {
	// A catalog with nothing in it behaves like a server that published no
	// metadata at all: every select degrades to a note.
	if cat == nil {
		cat = &catalog.Catalog{}
	}

	defaults := options.Defaults(cat)
	all := []field{
		textField(KeyGroupID, "Group", defaults.GroupID),
		textField(KeyArtifactID, "Artifact", defaults.ArtifactID),
		textField(KeyName, "Name", defaults.Name),
		textField(KeyPackageName, "Package", defaults.PackageName),
		selectField(KeyBuild, "Build", buildChoices(cat), options.BuildMaven),
		selectField(KeyLanguage, "Language", catalogChoices(cat.Language), cat.Language.Default),
		selectField(KeyPackaging, "Packaging", catalogChoices(cat.Packaging), cat.Packaging.Default),
		selectField(KeyJavaVersion, "Java", catalogChoices(cat.JavaVersion), cat.JavaVersion.Default),
		selectField(KeyBootVersion, "Boot", catalogChoices(cat.BootVersion), cat.BootVersion.Default),
	}

	fields := make([]field, 0, len(all))
	for _, field := range all {
		if _, answered := provided[field.key]; answered {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

func textField(key Key, label, initial string) field {
	return field{key: key, label: label, kind: fieldText, initial: initial}
}

// selectField offers choices in the order given and starts on preferred. A
// select without choices cannot be shown as a picker, so it becomes a note, and
// the value stays empty so the server applies its own default.
func selectField(key Key, label string, choices []choice, preferred string) field {
	if len(choices) == 0 {
		return field{key: key, label: label, kind: fieldNote, note: missingNote(label)}
	}
	return field{
		key:     key,
		label:   label,
		kind:    fieldSelect,
		initial: pick(choices, preferred),
		choices: choices,
	}
}

// pick returns preferred when it is on offer, and the first choice otherwise: a
// select must always start on one of its own values.
func pick(choices []choice, preferred string) string {
	for _, c := range choices {
		if c.value == preferred {
			return preferred
		}
	}
	if len(choices) == 0 {
		return ""
	}
	return choices[0].value
}

// missingNote is the line that replaces a select the metadata does not publish.
func missingNote(label string) string {
	return label + ": the server did not publish this option, so it will choose its own default."
}

// buildChoices offers only the metadata types that produce a project archive.
// The catalog keeps ids but drops each type's action and tags, so the inverse
// mapping in options is what tells an archive (`maven-project`) from a single
// build file (`maven-build`). Matching on the ids themselves would have to
// hardcode three names, and a "-project" suffix would silently lose
// `gradle-project-kotlin`; the mapping is the only correct signal.
func buildChoices(cat *catalog.Catalog) []choice {
	if cat == nil {
		return nil
	}

	choices := make([]choice, 0, len(cat.Type.Values))
	for _, value := range cat.Type.Values {
		build, ok := options.BuildName(value.ID)
		if !ok {
			continue
		}
		choices = append(choices, choice{value: build, label: value.Name})
	}
	return choices
}

// catalogChoices copies a select's values in the server's order. Only the id is
// the value: that is the string the server accepts back.
func catalogChoices(sel catalog.Select) []choice {
	choices := make([]choice, 0, len(sel.Values))
	for _, value := range sel.Values {
		choices = append(choices, choice{value: value.ID, label: value.Name})
	}
	return choices
}

// pointers maps every field key to the state field that stores it. It is the
// one place that binds a form row to formState, so adding a row means adding a
// line here and nothing else.
func (s *formState) pointers() map[Key]*string {
	return map[Key]*string{
		KeyGroupID:     &s.GroupID,
		KeyArtifactID:  &s.ArtifactID,
		KeyName:        &s.Name,
		KeyPackageName: &s.PackageName,
		KeyBuild:       &s.Build,
		KeyLanguage:    &s.Language,
		KeyPackaging:   &s.Packaging,
		KeyJavaVersion: &s.JavaVersion,
		KeyBootVersion: &s.BootVersion,
	}
}

// armPackage marks the current package value as derived, so the next change to
// the identifiers recomputes it. It is the first step of the derivation rule.
func (s *formState) armPackage() {
	s.packageAuto = true
	s.derivedPackage = s.PackageName
}

// followPackage repeats the package derivation after a form update. huh emits no
// "this field was edited" event, so an edit is detected by comparison: while the
// field still holds the value derivation last wrote, group and artifact keep
// winning; anything else means a human typed, and derivation stops for the rest
// of the session. An empty field re-arms it and is filled again immediately, so
// an empty package name never reaches the request.
//
// It reports whether it wrote a new value, because huh's input owns a second
// copy of the text and has to be resynchronised when that happens.
func (s *formState) followPackage() bool {
	switch {
	case s.PackageName == "":
		s.packageAuto = true
	case s.PackageName != s.derivedPackage:
		s.packageAuto = false
	}
	if !s.packageAuto {
		return false
	}

	derived := options.DerivePackageName(s.GroupID, s.ArtifactID)
	if derived == s.PackageName {
		return false
	}
	s.PackageName = derived
	s.derivedPackage = derived
	return true
}

// packageMarker is the description shown under the package field while its
// value is derived, and empty once a human owns it.
func (s *formState) packageMarker() string {
	if s.packageAuto {
		return autoMarker
	}
	return ""
}

// newForm renders the sections as one huh form with one group per section, so
// exactly one page of rows is on screen at a time and enter walks to the next
// section. It also returns the package input, which the update loop pushes the
// derived value and the (auto) marker into.
//
// A section with no rows is left out: huh panics on a group without fields
// because it always expects a selected one, and a section the flags answered
// completely has nothing to render. A form with no sections left has no form at
// all, and a Run that rendered it would be asking nothing anyway (US7.1; US7.2
// hides such a section and renumbers the rest).
func newForm(sections []section, values map[Key]*string, theme Theme) (*huh.Form, *huh.Input) {
	groups := make([]*huh.Group, 0, len(sections))
	var packageInput *huh.Input
	for i, section := range sections {
		if len(section.fields) == 0 {
			continue
		}

		widgets := make([]huh.Field, 0, len(section.fields))
		for _, f := range section.fields {
			widget := f.widget(values[f.key])
			if f.key == KeyPackageName {
				packageInput, _ = widget.(*huh.Input)
			}
			widgets = append(widgets, widget)
		}
		groups = append(groups, huh.NewGroup(widgets...).Title(section.title(i+1, len(sections))))
	}

	if len(groups) == 0 {
		return nil, nil
	}

	form := huh.NewForm(groups...).WithTheme(theme.huhForm())
	if focused := form.GetFocusedField(); focused != nil {
		// huh focuses the first row from Init, and only the program calls Init
		// once. A form rebuilt after a screen change never sees it, so focus the
		// first row here: typing has to reach it as soon as the form is back.
		focused.Focus()
	}
	return form, packageInput
}

// widget renders one row, editing the current value through value. The key is
// the row's identity: huh reports the focused field back by key, which is how
// the wrapper tells the first row from the rest.
func (f field) widget(value *string) huh.Field {
	switch f.kind {
	case fieldText:
		return huh.NewInput().Key(string(f.key)).Title(f.label).Value(value)
	case fieldSelect:
		choices := make([]huh.Option[string], 0, len(f.choices))
		for _, c := range f.choices {
			choices = append(choices, huh.NewOption(c.label, c.value))
		}
		return huh.NewSelect[string]().Key(string(f.key)).Title(f.label).Options(choices...).Value(value)
	default:
		return huh.NewNote().Title(f.label).Description(f.note)
	}
}

// optionsFromState is the only bridge from the form to the pipeline. It starts
// from the metadata defaults so the fields the form does not edit (description
// today, nothing else) keep their documented value, then lets the form win for
// every row it does edit.
//
// Dependencies stay empty until the dependency panel exists (VS3); today the
// only source for them is --deps, which is also why passing it skips the form.
func optionsFromState(cat *catalog.Catalog, state formState) options.Options {
	opts := options.Defaults(cat)

	opts.GroupID = state.GroupID
	opts.ArtifactID = state.ArtifactID
	opts.Name = state.Name
	opts.PackageName = state.PackageName
	opts.Build = state.Build
	opts.Language = state.Language
	opts.Packaging = state.Packaging
	opts.JavaVersion = state.JavaVersion
	opts.BootVersion = state.BootVersion

	return opts
}

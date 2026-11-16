package tui

// Tests for US5.3: g hands the focused dependency's first guide to the platform's
// opener, the hint only appears where the key would do something, and a failed
// launch leaves the address on screen. The opener is always stubbed: no test may
// start a browser. No test needs a terminal or a network.

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// webGuideHref is the first titled guide of the Web starter in the fixture.
const webGuideHref = "https://spring.io/guides/gs/rest-service/"

// stubOpener stands in for the platform's browser opener and puts it back after
// the test.
func stubOpener(t *testing.T, open func(string) error) {
	t.Helper()

	original := openURL
	openURL = open
	t.Cleanup(func() { openURL = original })
}

// TestGuideKeyOpensTheFirstGuide is the story's core: g on Spring Web opens the
// server's first titled guide, and a successful launch says nothing.
func TestGuideKeyOpensTheFirstGuide(t *testing.T) {
	cat := fixtureCatalog(t)

	var opened []string
	stubOpener(t, func(url string) error {
		opened = append(opened, url)
		return nil
	})

	screen := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")
	after := drive(t, screen, keyRunes("g"))

	if len(opened) != 1 {
		t.Fatalf("the opener was called %d times, want once", len(opened))
	}
	if want := firstGuideHref(mustDependency(t, cat, "web").Guides); opened[0] != want {
		t.Errorf("opened %q, want the first guide %q", opened[0], want)
	}
	if opened[0] != webGuideHref {
		t.Errorf("opened %q, want the fixture's %q", opened[0], webGuideHref)
	}
	if after.notice != "" {
		t.Errorf("a successful open left the notice %q, want none", after.notice)
	}
}

// TestGuideKeyIgnoresDependenciesWithoutGuides: no guides means nothing is opened,
// the hint is absent, and no message is invented.
func TestGuideKeyIgnoresDependenciesWithoutGuides(t *testing.T) {
	cat := fixtureCatalog(t)

	var opened []string
	stubOpener(t, func(url string) error {
		opened = append(opened, url)
		return nil
	})

	// dgs-codegen has no _links at all.
	tools := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Developer Tools")
	dgs := drive(t, tools, down())
	if got := focusedDependencyID(t, dgs); got != "dgs-codegen" {
		t.Fatalf("focus = %q, want dgs-codegen", got)
	}
	after := drive(t, dgs, keyRunes("g"))

	if len(opened) != 0 {
		t.Errorf("g opened %v for a dependency with no guides", opened)
	}
	if got := after.keyHints(); strings.Contains(got, "g guide") {
		t.Errorf("hints = %q, want no guide hint without a guide", got)
	}
	if after.notice != "" {
		t.Errorf("g on a dependency without guides left the notice %q", after.notice)
	}
	if got := len(after.list.VisibleItems()); got != len(tools.list.VisibleItems()) {
		t.Errorf("g changed the list: %d rows, want %d", got, len(tools.list.VisibleItems()))
	}

	// htmx has a link with no title, which is not a guide the client can open.
	web := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")
	untitled := web
	for range 16 {
		untitled = drive(t, untitled, down())
	}
	if got := focusedDependencyID(t, untitled); got != "htmx" {
		t.Fatalf("focus = %q, want htmx", got)
	}
	if got := firstGuideHref(mustDependency(t, cat, "htmx").Guides); got != "" {
		t.Fatalf("the fixture's htmx guide has an address %q, want the untitled case", got)
	}
	untitledAfter := drive(t, untitled, keyRunes("g"))
	if untitledAfter.notice != "" {
		t.Errorf("an untitled guide produced the notice %q", untitledAfter.notice)
	}
	if len(opened) != 0 {
		t.Errorf("g opened %v for an untitled guide", opened)
	}
}

// TestGuideHintFollowsTheFocus: the hint is present for Spring Web, gone for the
// row that has no guides, and the hints line stays inside 80 columns.
func TestGuideHintFollowsTheFocus(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")

	if !strings.Contains(screen.keyHints(), "g guide") {
		t.Fatalf("hints = %q, want the guide hint for Spring Web", screen.keyHints())
	}
	hints := strings.Split(screen.view(), "\n")
	if got := hints[len(hints)-1]; !strings.Contains(got, "g guide") {
		t.Errorf("the rendered hints line is %q, want the guide hint", got)
	}

	// webflux has one guide, spring-restclient none.
	moved := drive(t, drive(t, screen, down()), down())
	if got := focusedDependencyID(t, moved); got != "spring-restclient" {
		t.Fatalf("focus = %q, want spring-restclient", got)
	}
	if got := moved.keyHints(); strings.Contains(got, "g guide") {
		t.Errorf("hints = %q, want the guide hint gone", got)
	}

	for _, level := range []dependenciesModel{screen, moved, sizedScreen(t, cat, 80, 24)} {
		if width := ansi.StringWidth(level.keyHints()); width > 80 {
			t.Errorf("hints %q are %d cells wide, want at most 80", level.keyHints(), width)
		}
	}

	// The category root has no dependency, so it never advertises g.
	root := sizedScreen(t, cat, 80, 24)
	if got := root.keyHints(); strings.Contains(got, "g guide") {
		t.Errorf("the category root hints = %q, want no guide hint", got)
	}
}

// TestGuideFailureLeavesTheURLAndKeepsRunning: a failed launch is one line with
// the address, on the same frame, and the wizard carries on.
func TestGuideFailureLeavesTheURLAndKeepsRunning(t *testing.T) {
	cat := fixtureCatalog(t)

	stubOpener(t, func(string) error { return errors.New("xdg-open: executable file not found") })

	screen := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")
	failed := drive(t, screen, keyRunes("g"))

	if !strings.Contains(failed.notice, guideFailurePrefix) || !strings.Contains(failed.notice, webGuideHref) {
		t.Fatalf("notice = %q, want the prefix and the address", failed.notice)
	}
	if view := failed.view(); !strings.Contains(view, webGuideHref) {
		t.Errorf("the frame does not show the address; view:\n%s", view)
	}
	if got, want := frameLines(failed.view()), frameLines(screen.view()); got != want {
		t.Errorf("the failure changed the frame from %d lines to %d", want, got)
	}
	for _, r := range failed.statusLine() {
		if r > 127 {
			t.Errorf("the notice %q is not ASCII (found %q)", failed.notice, r)
			break
		}
	}

	// The wizard keeps working: the next key still moves the cursor.
	after := drive(t, failed, down())
	if got := focusedDependencyID(t, after); got != "webflux" {
		t.Errorf("the cursor did not move after the failure: focus = %q", got)
	}
	if got := frameLines(after.view()); got != frameLines(screen.view()) {
		t.Errorf("the frame changed shape after the cursor moved: %d lines", got)
	}

	// A level change makes the message stale, so it goes.
	back := drive(t, after, escKey())
	if back.notice != "" {
		t.Errorf("notice = %q after leaving the category, want it cleared", back.notice)
	}
	if strings.Contains(back.view(), webGuideHref) {
		t.Errorf("the address outlived its level; view:\n%s", back.view())
	}
}

// TestGuideOpenerPerPlatform pins the command each system gets, so the Linux,
// macOS and Windows paths are checked without running any of them.
func TestGuideOpenerPerPlatform(t *testing.T) {
	cases := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "linux", name: "xdg-open", args: []string{webGuideHref}},
		{goos: "darwin", name: "open", args: []string{webGuideHref}},
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", webGuideHref}},
		{goos: "freebsd", name: "xdg-open", args: []string{webGuideHref}},
	}

	for _, tc := range cases {
		name, args := openCommand(tc.goos, webGuideHref)
		if name != tc.name {
			t.Errorf("%s: command = %q, want %q", tc.goos, name, tc.name)
		}
		if strings.Join(args, " ") != strings.Join(tc.args, " ") {
			t.Errorf("%s: args = %q, want %q", tc.goos, args, tc.args)
		}
	}
}

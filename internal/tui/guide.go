package tui

// Opening a guide (US5.3): the metadata's first titled guide for the focused
// dependency goes to the platform's browser opener, and a failed start leaves the
// address on screen instead of ending the wizard.

import (
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// guideFailurePrefix introduces the line a failed open leaves behind: the address
// stays whole as far as the terminal is wide, so it can be copied.
const guideFailurePrefix = "Could not open the guide: "

// openURL hands a URL to the platform's opener. It is a variable so a test can
// stand in for the browser: nothing here may ever launch one.
var openURL = func(url string) error {
	name, args := openCommand(runtime.GOOS, url)
	return exec.Command(name, args...).Run()
}

// openCommand is how each platform opens a URL. Unknown systems get xdg-open,
// which is the convention there too.
func openCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// firstGuideHref is the address the g key opens: the server orders the guides, and
// an untitled or addressless link is skipped, the way the detail region skips it.
func firstGuideHref(guides []catalog.Link) string {
	for _, guide := range guides {
		if strings.TrimSpace(guide.Title) != "" && guide.Href != "" {
			return guide.Href
		}
	}
	return ""
}

// hasGuide reports whether the focused dependency has a guide to open, which is
// what makes the g hint honest.
func (d dependenciesModel) hasGuide() bool {
	dependency, ok := d.focusedDependency()
	return ok && firstGuideHref(dependency.Guides) != ""
}

// guideHint is the g hint, present only where the key would do something.
func (d dependenciesModel) guideHint() string {
	if !d.hasGuide() {
		return ""
	}
	return footerGap + "g guide"
}

// guideOpenedMsg is the opener's answer, delivered back to the update loop so the
// frame can report a failure without the list waiting on a browser.
type guideOpenedMsg struct {
	href string
	err  error
}

// openFocusedGuide starts the platform opener for the focused dependency's first
// guide, as a command: a slow browser launch must not freeze the list.
func (d dependenciesModel) openFocusedGuide() (dependenciesModel, tea.Cmd) {
	dependency, ok := d.focusedDependency()
	if !ok {
		return d, nil
	}

	href := firstGuideHref(dependency.Guides)
	if href == "" {
		return d, nil
	}

	open := openURL
	return d, func() tea.Msg {
		return guideOpenedMsg{href: href, err: open(href)}
	}
}

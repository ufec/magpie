//go:build windows && !nogui

package gui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// application.New is a singleton (a second call returns the first App), so
// one test binary can hold only one Options. The cases that need different
// ones (a runtime Wails is pointed at) run in a child process of this
// binary, told apart by MAGPIE_TEST_WEBVIEW2.
const testWebView2Env = "MAGPIE_TEST_WEBVIEW2"

// TestWebView2Main runs in the child: it reports what webView2Installed
// says for the Options the parent asked for, and exits.
func TestWebView2Main(t *testing.T) {
	mode := os.Getenv(testWebView2Env)
	if mode == "" {
		t.Skip("not the child process")
	}
	opts := application.Options{Name: "webview2-check"}
	switch mode {
	case "edge":
		edge := edgeWebView()
		if edge == "" {
			os.Exit(3) // no Edge to point at
		}
		opts.Windows.WebviewBrowserPath = edge
	}
	if webView2Installed(application.New(opts)) {
		os.Exit(0)
	}
	os.Exit(1)
}

// webView2Child runs TestWebView2Main in a child process in mode, and
// reports its exit code: 0 installed, 1 not, 3 nothing to point at.
func webView2Child(t *testing.T, mode string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWebView2Main$", "-test.v")
	cmd.Env = append(os.Environ(), testWebView2Env+"="+mode)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if ee.ExitCode() == 3 {
		t.Skipf("no Edge EBWebView on this machine to point Wails at\n%s", out)
	}
	return ee.ExitCode()
}

// The check is Wails' own: Env.Info()'s PlatformInfo["WebView2"], the same
// lookup a window's WebView2 environment makes. On a machine with no
// runtime it is "" (the reported case), and on one with a runtime it is the
// version.
func TestWebView2Installed(t *testing.T) {
	if got := webView2Child(t, "none"); got != 1 {
		t.Fatalf("no runtime: child exit %d, want 1 (not installed)", got)
	}
}

// A machine that does have a runtime is reported installed: Wails is
// pointed at Edge's own EBWebView, which is a real runtime it accepts.
func TestWebView2InstalledWithRuntime(t *testing.T) {
	if got := webView2Child(t, "edge"); got != 0 {
		t.Fatalf("runtime present: child exit %d, want 0 (installed)", got)
	}
}

// The notice names what is missing, one install command, the address the
// Help button opens, and the two buttons' labels, in each language the app
// speaks.
func TestWebView2NoticeText(t *testing.T) {
	for _, lang := range []string{"en", "zh", "zh-TW", "ja", "de"} {
		title, body, help, close := webView2NoticeText(lang)
		if title == "" || body == "" || help == "" || close == "" {
			t.Fatalf("%s: empty title, body or button label (%q %q %q %q)", lang, title, body, help, close)
		}
		if help == close {
			t.Errorf("%s: both buttons read %q", lang, help)
		}
		if !strings.Contains(body, webView2Command) {
			t.Errorf("%s: no install command in %q", lang, body)
		}
		// the command stands on a line of its own, and the box says
		// nothing about downloading: the Help button is where the rest is
		if !strings.HasSuffix(body, "\r\n\r\n"+webView2Command) {
			t.Errorf("%s: the command is not alone on the last line: %q", lang, body)
		}
		if strings.Contains(body, "http") {
			t.Errorf("%s: the body carries an address; Help opens the guide instead: %q", lang, body)
		}
	}
	// the reader's language is really used, not the English fallback
	if _, body, help, close := webView2NoticeText("zh"); !strings.Contains(body, "运行时") || help != "帮助" || close != "关闭" {
		t.Errorf("zh: not Chinese: body=%q help=%q close=%q", body, help, close)
	}
	// an unknown language falls back to English rather than to nothing
	if title, _, help, close := webView2NoticeText("pt"); title != "magpie needs WebView2" || help != "Help" || close != "Close" {
		t.Errorf("pt: title=%q help=%q close=%q, want the English fallback", title, help, close)
	}
}

// The buttons map to Windows' ids the way the box's return value needs:
// Help is IDOK (1), Close is IDCANCEL (2), because Windows returns
// IDCANCEL for Escape and for the title bar's X as well — those must
// close, not open the page.
func TestWebView2ButtonsAreWindowsIDs(t *testing.T) {
	if idOK != 1 || idCancel != 2 {
		t.Fatalf("idOK=%d idCancel=%d, want 1 and 2", idOK, idCancel)
	}
}

// Help opens magpie's own guide, which explains the runtime; magpie never
// downloads it itself.
func TestWebView2HelpIsMagpiesGuide(t *testing.T) {
	if !strings.HasPrefix(webView2Docs, "https://usemagpie.ai/docs/") {
		t.Errorf("webView2Docs = %q, want a page of magpie's own guide", webView2Docs)
	}
	if !strings.HasSuffix(webView2Docs, "#webview2") {
		t.Errorf("webView2Docs = %q, want the section that explains WebView2", webView2Docs)
	}
}

// webView2Check is what Run asks: with no runtime the notice is shown and
// the launch ends; with one, nothing is shown and it goes on.
func TestWebView2Check(t *testing.T) {
	oldNotice := webView2Notice
	t.Cleanup(func() { webView2Notice = oldNotice })
	var shown []string
	webView2Notice = func(title, body, help, close string) {
		shown = append(shown, title+"\n"+body+"\n"+help+"\n"+close)
	}

	// this machine has no runtime, so the notice is shown and the launch ends
	app := application.New(application.Options{Name: "webview2-check"})
	if webView2Check(app) {
		t.Fatal("no runtime: webView2Check said to go on")
	}
	if len(shown) != 1 {
		t.Fatalf("no runtime: the notice was shown %d times, want once", len(shown))
	}
	if !strings.Contains(shown[0], webView2Command) {
		t.Errorf("no runtime: the notice does not say how to install it: %q", shown[0])
	}
}

// edgeWebView is Edge's own EBWebView folder, the newest one installed, or
// "" when Edge isn't here. Wails accepts it as a runtime (pointed at with
// WindowsOptions.WebviewBrowserPath), which is what the child process uses
// to stand in for an installed runtime without one being installed. magpie
// itself never points Wails at it: Edge's copy isn't the Evergreen runtime,
// and Wails refuses it as one (the environment fails with "The request is
// not supported").
func edgeWebView() string {
	roots := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application"),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		var versions []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, e.Name(), "EBWebView", "x64", "EmbeddedBrowserWebView.dll")); err == nil {
				versions = append(versions, e.Name())
			}
		}
		if len(versions) > 0 {
			sort.Strings(versions)
			return filepath.Join(root, versions[len(versions)-1])
		}
	}
	return ""
}

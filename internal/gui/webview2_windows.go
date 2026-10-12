//go:build windows && !nogui

package gui

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// webView2Installed asks Wails itself whether the runtime its windows need
// is there, rather than repeating Wails' registry lookup here: Env.Info()
// runs the same check a window's WebView2 environment does, so it stays
// right as Wails changes. On this machine it reports "" when the runtime is
// missing, and the version otherwise.
//
// It is read before Run, which is safe: the lookup reads the registry and
// makes no window and no WebView2 environment.
func webView2Installed(app *application.App) bool {
	info := app.Env.Info()
	v, ok := info.PlatformInfo["WebView2"]
	if !ok {
		// Wails didn't report one at all (a version that stops setting
		// the key). Nothing says the runtime is missing, and wrongly
		// refusing to start a magpie that would have worked is worse
		// than the silent exit this notice exists to fix.
		return true
	}
	return fmt.Sprint(v) != ""
}

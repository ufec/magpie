//go:build !windows && !nogui

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

// webView2Check has nothing to check: the Mac's WKWebView and Linux's
// WebKitGTK come with the system, not from a runtime that can be missing,
// so the app always goes on. The parameter matches the Windows one, whose
// check needs the app.
func webView2Check(*application.App) bool { return true }

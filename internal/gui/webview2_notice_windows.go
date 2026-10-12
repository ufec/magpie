//go:build windows && !nogui

package gui

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yetone/magpie/internal/settings"
)

// A machine with no WebView2 runtime gets no window at all: Wails reports
// the failure and exits, and the desktop app has no console for that report
// to land in. So magpie tells the reader itself, and says what to install.
// `magpie web` and `magpie tui` are named too: they need no WebView2 and are
// a way in right now.
//
// The notice is Windows' own message box, with its buttons relabelled to
// Help and Close (webView2NoticeProc, a CBT hook): the box is the system's,
// drawn and sized by it, and only the two buttons' text is magpie's. Help
// opens magpie's own guide; Close ends the launch.

// webView2Command installs the runtime; webView2Docs is magpie's own guide,
// which the Help button opens and where the runtime is explained.
const (
	webView2Command = "winget install Microsoft.EdgeWebView2Runtime"
	webView2Docs    = "https://usemagpie.ai/docs/start#webview2"
)

// webView2Translations are the notice's words in the languages the app
// speaks; the command is the same in every one. help and close are the two
// buttons' labels.
var webView2Translations = map[string]struct{ title, body, help, close string }{
	"zh": {
		title: "magpie 需要 WebView2",
		body:  "magpie 的窗口需要 Microsoft Edge WebView2 运行时，这台电脑上没有找到它。\r\n\r\n可以用下面的命令安装：",
		help:  "帮助",
		close: "关闭",
	},
	"zh-TW": {
		title: "magpie 需要 WebView2",
		body:  "magpie 的視窗需要 Microsoft Edge WebView2 執行階段，這台電腦上找不到它。\r\n\r\n可以用下面的指令安裝：",
		help:  "說明",
		close: "關閉",
	},
	"ja": {
		title: "magpie には WebView2 が必要です",
		body:  "magpie のウィンドウには Microsoft Edge WebView2 ランタイムが必要ですが、この PC には見つかりません。\r\n\r\n次のコマンドでインストールできます：",
		help:  "ヘルプ",
		close: "閉じる",
	},
	"de": {
		title: "magpie braucht WebView2",
		body:  "Die Fenster von magpie brauchen die Microsoft Edge WebView2-Runtime; auf diesem PC ist sie nicht zu finden.\r\n\r\nMit diesem Befehl lässt sie sich installieren:",
		help:  "Hilfe",
		close: "Schließen",
	},
}

// webView2NoticeText is the notice in lang: the whole text as the message
// box shows it, and the two buttons' labels.
func webView2NoticeText(lang string) (title, body, help, close string) {
	title = "magpie needs WebView2"
	body = "magpie's windows need the Microsoft Edge WebView2 runtime, and this PC hasn't got it.\r\n\r\n" +
		"It can be installed with the command below:" + "\r\n\r\n" + webView2Command
	help, close = "Help", "Close"
	if t, ok := webView2Translations[lang]; ok {
		title, body, help, close = t.title, t.body+"\r\n\r\n"+webView2Command, t.help, t.close
	}
	return title, body, help, close
}

// webView2Missing is the notice in the reader's language, as the tray menu
// and the page pick theirs.
func webView2Missing() (title, body, help, close string) {
	return webView2NoticeText(trayLang(settings.Load().Lang, systemLang))
}

// webView2Notice shows the notice. It is a variable so tests can stand in
// for it.
var webView2Notice = showWebView2Notice

// webView2Check is what Run asks before it starts the app: a Windows
// desktop build with no WebView2 runtime has no window to show, so the
// reader is told and the launch ends without Run. The answer is Wails' own
// (webView2Installed), so it doesn't drift from what Wails needs.
func webView2Check(app *application.App) bool {
	if webView2Installed(app) {
		return true
	}
	title, body, help, close := webView2Missing()
	webView2Notice(title, body, help, close)
	return false
}

// Windows' own message box and its button ids.
const (
	mbOKCancel = 0x00000001
	mbIconErr  = 0x00000010
	mbSetFg    = 0x00010000
	mbTopmost  = 0x00040000

	idOK     = 1
	idCancel = 2

	whCBT        = 5
	hCBTActivate = 5
	dmSetDefID   = 0x0401
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	setWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	callNextHookEx      = user32.NewProc("CallNextHookEx")
	unhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	setWindowTextW      = user32.NewProc("SetWindowTextW")
	getDlgItem          = user32.NewProc("GetDlgItem")
	sendMessageW        = user32.NewProc("SendMessageW")
	getModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	getCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
)

// noticeButtons are the two labels the hook writes, set before the box is
// shown. One notice runs at a time and the process ends with it, so they
// are package state rather than threaded through the callback.
var noticeButtons struct{ help, close string }

// noticeHook is the CBT hook procedure, kept so the callback is never
// collected while Windows still calls it.
var noticeHook = syscall.NewCallback(webView2NoticeProc)

// showWebView2Notice shows Windows' message box and relabels its two buttons
// as Help and Close. Help opens magpie's guide; Close — and the box's own
// close, Escape and Alt+F4 — returns without opening anything.
//
// The box is MB_OKCANCEL so it has exactly two buttons to relabel. A CBT
// hook on this thread sees the box as it is activated, before it is drawn,
// which is where the labels are changed: OK becomes Help and Cancel becomes
// Close, and Cancel is made the default so Enter, Escape and the title
// bar's X all close. The ids are not changed, so the return value is still
// which button was pressed: IDOK means Help, anything else means Close —
// which is why Close is the one on Cancel, since Windows returns IDCANCEL
// for Escape and for the X as well.
func showWebView2Notice(title, body, help, close string) {
	noticeButtons.help, noticeButtons.close = help, close

	hInst, _, _ := getModuleHandleW.Call(0)
	tid, _, _ := getCurrentThreadID.Call()
	hook, _, _ := setWindowsHookExW.Call(whCBT, noticeHook, hInst, tid)
	defer func() {
		if hook != 0 {
			unhookWindowsHookEx.Call(hook)
		}
	}()

	flags := uint32(mbOKCancel | mbIconErr | mbSetFg | mbTopmost)
	r, _, _ := messageBoxW.Call(uintptr(consoleWindow()),
		uintptr(unsafe.Pointer(u16(body))), uintptr(unsafe.Pointer(u16(title))), uintptr(flags))
	if int32(r) == idOK {
		// Help: magpie's own guide
		openWebView2Help()
	}
}

// webView2NoticeProc is the CBT hook: when the message box is activated, it
// relabels the two buttons. OK (drawn first) becomes Help and Cancel (drawn
// second, where Close belongs) becomes Close; Cancel is made the default,
// so Enter, Escape and the X all close rather than open the page.
func webView2NoticeProc(nCode, wParam, lParam uintptr) uintptr {
	if nCode == hCBTActivate {
		if b, _, _ := getDlgItem.Call(wParam, idOK); b != 0 {
			setWindowTextW.Call(b, uintptr(unsafe.Pointer(u16(noticeButtons.help))))
		}
		if b, _, _ := getDlgItem.Call(wParam, idCancel); b != 0 {
			setWindowTextW.Call(b, uintptr(unsafe.Pointer(u16(noticeButtons.close))))
		}
		sendMessageW.Call(wParam, dmSetDefID, idCancel, 0)
	}
	r, _, _ := callNextHookEx.Call(0, nCode, wParam, lParam)
	return r
}

// openWebView2Help opens magpie's own guide in the system browser, as the
// app opens its other links. The guide says how to install the runtime;
// magpie itself downloads nothing.
func openWebView2Help() {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shellExecuteW := shell32.NewProc("ShellExecuteW")
	shellExecuteW.Call(0,
		uintptr(unsafe.Pointer(u16("open"))),
		uintptr(unsafe.Pointer(u16(webView2Docs))),
		0, 0, 1 /*SW_SHOWNORMAL*/)
}

// messageBoxW is Windows' MessageBoxW.
var messageBoxW = user32.NewProc("MessageBoxW")

// consoleWindow is the console magpie was started from, or 0 when it has
// none (the desktop app's usual start from Explorer).
func consoleWindow() uintptr {
	h, _, _ := getConsoleWindow.Call()
	return h
}

var getConsoleWindow = kernel32.NewProc("GetConsoleWindow")

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

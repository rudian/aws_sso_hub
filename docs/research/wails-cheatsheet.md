# Wails v3.1.0 cheat sheet (AWS SSO Hub)

Copy from this file and do not guess. All code below was compiled with go1.27.3 and Wails v3.1.0 on 2026-11-04, unless it is marked UNVERIFIED. The background and evidence are in `docs/research/wails.md`.

Names used throughout:
- Go module: `awsssohub`
- Taskfile `APP_NAME` (binary and bundle file name): `aws-sso-hub`
- Product name: `AWS SSO Hub`
- Bundle id / single-instance id: `com.awsssohub.app`. This is a placeholder: pick the final one once and never change it.

---

## 0. Versions and install

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0   # → ~/go/bin/wails3 (must be on PATH: the Taskfiles call `wails3`)
wails3 version                                              # v3.1.0
```

| Where | Pin |
|---|---|
| `go.mod` | `require github.com/wailsapp/wails/v3 v3.1.0` (the template writes `go 1.26.0`; leave it) |
| `frontend/package.json` | `"@wailsio/runtime": "3.1.0"` (exact) |
| Frontend | `react`/`react-dom` `^19.3.1`, `@types/react`/`@types/react-dom` `^19.3.0`, `typescript` `^7.0.2` (fallback `~5.9.3`), `vite` `^8.3.5`, `@vitejs/plugin-react` `^6.2.1`, `lucide-react` `^1.67.0`, `clsx` `^2.1.1` |

Never run `wails3 update` or `npm update @wailsio/runtime`. The CLI, the Go module and the npm runtime must stay on the same version.

---

## 1. Scaffold into the existing project root

`wails3 init -d X` creates `X/<name>`, so init in a temp folder and copy the result:

```bash
TMP=$(mktemp -d)
cd "$TMP" && wails3 init -n aws-sso-hub -t react -mod awsssohub \
  -productname "AWS SSO Hub" -productidentifier com.awsssohub.app \
  -productversion 0.1.0 -productcompany "AWS SSO Hub" -productdescription "AWS IAM Identity Center and CodeCommit helper"
rsync -a "$TMP/aws-sso-hub/" ~/Documents/code/multi_aws_account/
```

## 2. Required post-scaffold edits (all VERIFIED in the spike)

1. **Remove mobile targets.** They break `GOOS=windows go build ./...`.
   ```bash
   rm -rf build/ios build/android
   sed -i '' '/ios: .\/build\/ios\/Taskfile.yml/d; /android: .\/build\/android\/Taskfile.yml/d' Taskfile.yml
   ```
2. **macOS minimum 13.0** (Go 1.27 targets macOS 13):
   - `build/darwin/Taskfile.yml` → `CGO_CFLAGS: "-mmacosx-version-min=13.0"`, `CGO_LDFLAGS: "-mmacosx-version-min=13.0"`, `MACOSX_DEPLOYMENT_TARGET: "13.0"`.
   - `build/darwin/Info.plist` and `build/darwin/Info.dev.plist` → `<key>LSMinimumSystemVersion</key><string>13.0.0</string>`.
3. **Icons from one PNG.** Without this, the macOS icon stays the Wails logo.
   ```bash
   rm -rf build/appicon.icon build/darwin/Assets.car
   sed -i '' 's# -iconcomposerinput appicon.icon -macassetdir darwin##; /- "appicon.icon"/d' build/Taskfile.yml
   ```
   Then delete the `<key>CFBundleIconName</key><string>appicon</string>` pair from `build/darwin/Info.plist` and `build/darwin/Info.dev.plist`.
4. Delete `frontend/.npmrc` (npm warns on every run; pnpm would block `@wailsio/runtime` for 7 days after its release).
5. Delete template assets: `frontend/public/{Inter-Medium.ttf,bg-desktop.jpg,bg-mobile.jpg,react.svg,wails.png,style.css}` and `frontend/Inter Font License.txt`. Remove the `<link rel="stylesheet" href="/style.css">`, `<div class="bg">` and `/wails.png` favicon lines from `frontend/index.html`.
6. `frontend/tsconfig.json`: `"target": "ES2022"`, `"lib": ["ES2022", "DOM", "DOM.Iterable"]`. ES2022 is needed for `Error.cause`.
7. Delete `greetservice.go`, and remove `GreetService` and the `time` ticker from `main.go` (replace it with §3).
8. `.gitignore`: add `build/windows/nsis/MicrosoftEdgeWebview2Setup.exe`. **Do commit `frontend/bindings/`.** The frontend builds and type-checks only when bindings exist.
9. Never run `wails3 update build-assets` / `wails3 task common:update:build-assets`. They overwrite the plists (back to 12.0.0) and re-create `build/ios`. Edit the build files by hand (§13).

Recommended layout:

```
main.go                       // app + window + tray wiring only
internal/app/...              // bound services (one struct per area: AccountService, RepoService, GitService, SettingsService, EnvService)
internal/execx/               // PATH + hidden-console exec helper (§12)
internal/...                  // pure logic (ini, sso, git) with unit tests, no Wails imports
frontend/src/api/             // facade: real bindings or mocks (§10)
frontend/bindings/            // generated, committed
build/                        // Wails build assets
```

---

## 3. `main.go` skeleton (compiled and run on macOS)

```go
package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"awsssohub/internal/app"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// Typed custom events: register in init() with constant names (the generator scans these calls).
func init() {
	application.RegisterEvent[app.LoginStatus]("auth:status")
	application.RegisterEvent[[]string]("app:second-instance")
}

func main() {
	accounts := app.NewAccountService()
	var mainWindow *application.WebviewWindow

	wapp := application.New(application.Options{
		Name:        "AWS SSO Hub",
		Description: "AWS IAM Identity Center and CodeCommit helper",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(accounts),
			// application.NewService(repos), ...
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true, // default: closing the window quits
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: false,
			WebviewUserDataPath: webviewDataDir(), // stable even if the exe is renamed (Windows-only option)
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.awsssohub.app",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if mainWindow != nil {
					mainWindow.Show()
					mainWindow.Restore()
					mainWindow.Focus()
					mainWindow.EmitEvent("app:second-instance", data.Args)
				}
			},
		},
	})

	mainWindow = wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "AWS SSO Hub",
		Width:            1200,
		Height:           780,
		MinWidth:         960,
		MinHeight:        640,
		URL:              "/",
		BackgroundColour: application.NewRGB(248, 249, 251), // matches the light theme page bg; avoids a white flash
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset, // traffic lights over content
			InvisibleTitleBarHeight: 52,                                 // top 52px drags the window
			Backdrop:                application.MacBackdropNormal,      // translucent needs -tags private_mac_apis: don't
		},
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault, // native title bar follows OS light/dark
			// Mica: BackdropType: application.Mica, AND window BackgroundType: application.BackgroundTypeTranslucent,
			// AND a transparent CSS body. Only applied when BackgroundType is Translucent (SOURCE). Not recommended for v1.
		},
	})

	// Optional "keep running in tray" mode (setting, default off). See §9.
	keepInTray := false
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if keepInTray {
			mainWindow.Hide()
			e.Cancel()
		}
	})

	setupTray(wapp, mainWindow, accounts) // §9; on darwin use a template icon

	if err := wapp.Run(); err != nil {
		log.Fatal(err)
	}
}

// webviewDataDir returns %APPDATA%\AWS SSO Hub\WebView2 on Windows, "" elsewhere (ignored off Windows).
func webviewDataDir() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if dir, err := os.UserConfigDir(); err == nil { // %APPDATA% on Windows
		return filepath.Join(dir, "AWS SSO Hub", "WebView2")
	}
	return ""
}
```

Notes:
- `application.Get()` returns the `*application.App` from anywhere (use it in `ServiceStartup`).
- `OnShutdown` (option and `app.OnShutdown(func(){})`) exists, but on Windows it is **not** called when the last window is closed with X (issue #6368). Do not rely on it.
- Managers on `*App`: `Window`, `Dialog`, `Event`, `Browser`, `Clipboard`, `SystemTray`, `Menu`, `Env`, `Screen`, `ContextMenu`, `KeyBinding`, `Autostart`, `GlobalShortcut`.
- With no custom menu set, Wails installs `DefaultApplicationMenu()` (App, File, Edit, View, Window, Help). The Edit menu is what makes Cmd+C/V/X/A work in inputs on macOS. If you ever set a custom menu (`wapp.Menu.Set(m)`), include `m.AddRole(application.EditMenu)`.

---

## 4. Services (bound Go code)

```go
package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type AccountService struct{ app *application.App }

func NewAccountService() *AccountService { return &AccountService{} }

// Optional lifecycle hooks (interfaces application.ServiceStartup / ServiceShutdown):
func (s *AccountService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.app = application.Get()
	return nil
}
func (s *AccountService) ServiceShutdown() error { return nil }

// Every EXPORTED method becomes callable from JS. Unexported ones are not bound.
// A first parameter of type context.Context is injected by Wails, hidden from TS,
// and cancelled when the JS promise is cancelled (p.cancel()).
func (s *AccountService) Login(ctx context.Context, session string) (LoginStatus, error) { /* ... */ return LoginStatus{}, nil }
```

Registration: `application.NewService(ptr)` in `Options.Services`, or `wapp.RegisterService(application.NewService(ptr))` before `Run`.

### 4.1 Go → TS type mapping (VERIFIED from the generated output, mode `-i` = interfaces)

| Go | Generated TS |
|---|---|
| `string`, `bool`, `int`/`float64` | `string`, `boolean`, `number` (int64 also becomes `number`, so keep AWS account IDs as `string`) |
| `[]T` | `T[] \| null`. A nil slice arrives as `null`: **always return `make([]T, 0)`** and still use `?? []` in TS |
| `map[string]V` | `{ [_ in string]?: V } \| null` |
| `*T` | `T \| null` |
| `time.Time` | `string` (RFC 3339) |
| struct field with `json:"x"` | property `"x"`. `omitempty` → optional `"x"?`. `json:"-"` and unexported fields are omitted |
| `type K string` + `const (A K = "a"; B K = "b")` | `export enum K { $zero = "", A = "a", B = "b" }`. Members keep the **Go const names** (`AccountKindSSO`), not shortened names |
| `type S int` + iota consts | `export enum S { $zero = 0, SeverityInfo = 0, ... }` |
| result `(T, error)` | `$CancellablePromise<T>` |
| result `error` only | `$CancellablePromise<void>` |
| result `(A, B, error)` | `$CancellablePromise<[A, B]>` |
| param `context.Context` (first) | omitted |
| A struct used only as an error (not in any signature) | **not generated**: declare a TS interface by hand |

Doc comments on methods and types are copied into the TS output.

### 4.2 Errors

```go
// Structured error: JSON fields reach JS as error.cause.
type AppError struct {
	Code    string `json:"code"`    // e.g. "E_SSO_EXPIRED", "E_AWS_CLI_MISSING"
	Message string `json:"message"` // English fallback; the UI translates by code
	Detail  string `json:"detail,omitempty"`
}
func (e *AppError) Error() string { return e.Message }

return nil, &AppError{Code: "E_NOT_FOUND", Message: "profile not found"}
```

What JS receives (VERIFIED):
- `&AppError{...}` → a `Call.RuntimeError` with `message` = `Error()` and `cause` = `{"code":"E_NOT_FOUND","message":"profile not found"}`.
- `errors.New("x")` → a `Call.RuntimeError` with `message` = `"x"` and `cause` = `{}`.
- A panic in a bound method is recovered and becomes a `RuntimeError` (`"<FQN>: panic: ..."`).
- Wrong argument types → `TypeError`.
- Unknown method → `ReferenceError`.

`Options.MarshalError func(error) []byte` (app-wide) or `ServiceOptions.MarshalError` can customise the cause JSON. It is not needed if you return `*AppError`.

**Never put secrets in error messages or causes.**

---

## 5. Binding generation

The build task runs it automatically (`common:generate:bindings`) before every `wails3 build`/`package`/`dev`. To run it by hand:

```bash
wails3 generate bindings -clean=true -ts -i     # first run ~36 s, cached ~1.4 s
```

- Output root: `frontend/bindings/`.
- Per Go package: `frontend/bindings/<module path>/<package path>/`.
  - Module `awsssohub`, package `internal/app` → `frontend/bindings/awsssohub/internal/app/`.
  - That folder holds `<lowercased struct name>.ts` (e.g. `accountservice.ts`), `models.ts` (interfaces + enums) and `index.ts` (re-exports).
- Events: `frontend/bindings/events/index.ts` exports a constant per registered event (`"auth:status"` → `AuthStatus`, `"app:second-instance"` → `AppSecondInstance`). Typed payloads are in `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts`.
- Method IDs are hashes of the fully qualified name. If you rename a package, struct or method, regenerate (the build does it).

TS import paths (from `frontend/src/<file>.tsx`; add one `../` per extra folder depth):

```ts
import { AccountService, AccountKind } from '../bindings/awsssohub/internal/app'        // service namespace + enums
import type { Account, LoginStatus } from '../bindings/awsssohub/internal/app'          // interfaces
import * as AccountServiceFns from '../bindings/awsssohub/internal/app/accountservice'  // module form (used for typeof in mocks)
import { AuthStatus } from '../bindings/events'                                          // event name constants
import { Events, Call, Dialogs, Browser, Clipboard, System, CancellablePromise } from '@wailsio/runtime'
```

---

## 6. Calling Go from TS

```ts
const accounts = (await AccountService.ListAccounts()) ?? []

try {
  await AccountService.Fail()
} catch (e: unknown) {
  if (e instanceof Call.RuntimeError) {
    const cause = e.cause as AppError | undefined   // AppError: hand-written TS interface { code; message; detail? }
    showError(cause?.code ?? 'E_UNKNOWN', e.message)
  } else {
    throw e   // TypeError / ReferenceError = programming bug
  }
}

// Cancellable long call (Go ctx is cancelled):
const p = AccountService.Login('acme-prod')   // CancellablePromise<LoginStatus>
cancelButton.onclick = () => p.cancel()
// or: p.cancelOn(abortController.signal)
```

---

## 7. Events

Go → JS:
```go
// Registered type must match EXACTLY (LoginStatus, not *LoginStatus) – "Data types are matched exactly" (SOURCE).
s.app.Event.Emit("auth:status", LoginStatus{Session: "acme-prod", LoggedIn: true})   // to all windows
mainWindow.EmitEvent("app:second-instance", data.Args)                                // emitted from a window
```
```ts
useEffect(() => {
  const off = Events.On(AuthStatus, (ev) => setStatus(ev.data))   // ev.data typed as LoginStatus
  return off                                                      // On() returns the unsubscribe fn
}, [])
// Also: Events.Once(name, cb), Events.OnMultiple(name, cb, n), Events.Off(name), Events.OffAll()
```

JS → Go:
```ts
await Events.Emit('ui:ready', null)   // the name must be registered with RegisterEvent if you want typing
```
```go
off := wapp.Event.On("ui:ready", func(e *application.CustomEvent) { _ = e.Data })
defer off()
```

Prefer **bound methods** for request/response and **events** only for push notifications (login state changes, expiry ticks, scan progress).

---

## 8. Native dialogs, browser, clipboard, file manager

### 8.1 Folder picker (Go side, compiled; dialog not clicked through)

```go
// Returns "" (and nil error) when the user cancels – treat "" as cancel.
func (s *SystemService) ChooseFolder(title string) (string, error) {
	return s.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		PromptForSingleSelection()
}
```

JS alternative (no Go code needed; types from the runtime):
```ts
const dir = await Dialogs.OpenFile({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: t('repos.pickFolder') })
if (dir) { /* string */ }
```

### 8.2 Confirmation

**Recommended:** an in-app React modal (translated, styled, shows a diff or details, can use the account colour). Use it for every destructive action.

Native options if ever needed:
```ts
const label = await Dialogs.Question({
  Title: t('confirm.title'), Message: t('confirm.body'),
  Buttons: [{ Label: t('common.delete') }, { Label: t('common.cancel'), IsCancel: true, IsDefault: true }],
})
if (label === t('common.delete')) { /* confirmed */ }
```
```go
// Go-side blocking confirm (compiled; UNVERIFIED that the cancel callback fires on Esc on every OS – that is why the React modal is preferred).
func (s *SystemService) Confirm(title, message, yes, no string) bool {
	answer := make(chan bool, 1)
	d := s.app.Dialog.Question().SetTitle(title).SetMessage(message)
	d.AddButton(yes).OnClick(func() { answer <- true })
	cancel := d.AddButton(no).OnClick(func() { answer <- false })
	d.SetDefaultButton(cancel)
	d.SetCancelButton(cancel)
	if w := s.app.Window.Current(); w != nil { d.AttachToWindow(w) }
	d.Show()
	return <-answer
}
```
Other message dialogs: `s.app.Dialog.Info()`, `.Warning()`, `.Error()`, then `.SetTitle(...).SetMessage(...).Show()`.

### 8.3 Open URL, clipboard, reveal in Finder/Explorer

```go
s.app.Browser.OpenURL("https://d-1234567890.awsapps.com/start")   // default browser; use this for the SSO verification URL
s.app.Clipboard.SetText(cmd)                                       // returns bool
text, ok := s.app.Clipboard.Text()
s.app.Env.OpenFileManager("/path/to/repo", false)                 // true = select the file instead of opening the folder
s.app.Env.IsDarkMode()                                             // bool
```
```ts
await Browser.OpenURL(url)
await Clipboard.SetText(text)
```
`<a href>` links inside the webview do not open the system browser. Always route external links through `Browser.OpenURL`.

---

## 9. System tray / menu bar (optional feature; compiled + ran on macOS; Windows issue #6378)

```go
func setupTray(wapp *application.App, win *application.WebviewWindow, accounts *app.AccountService) {
	tray := wapp.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayTemplatePNG) // monochrome black+alpha PNG, ~44x44 (22pt @2x); macOS tints it
	} else {
		tray.SetIcon(trayColorPNG) // 32x32 or 64x64 PNG
	}
	tray.SetTooltip("AWS SSO Hub")

	menu := wapp.NewMenu()
	rebuild := func(sessions []string) {
		menu.Clear()
		for _, name := range sessions {
			name := name
			menu.Add("SSO Login: " + name).OnClick(func(*application.Context) {
				go accounts.Login(wapp.Context(), name) // never block the menu callback
			})
		}
		menu.AddSeparator()
		menu.Add("Open AWS SSO Hub").OnClick(func(*application.Context) { win.Show(); win.Focus() })
		menu.Add("Quit").OnClick(func(*application.Context) { wapp.Quit() })
		menu.Update() // REQUIRED after changing items; no-op before Run()
	}
	rebuild([]string{"acme-prod"})
	tray.SetMenu(menu)

	// Rebuild whenever state changes:
	wapp.Event.On("auth:status", func(*application.CustomEvent) { rebuild(currentSessionNames()) })
}
```
Other `SystemTray` methods: `SetLabel` (macOS text next to the icon), `SetDarkModeIcon`, `OnClick`, `OnRightClick`, `AttachWindow(w)`, `Show`, `Hide`, `Destroy`. Menu item methods: `SetLabel`, `SetEnabled`, `SetChecked`, `SetHidden`, `SetBitmap`, `SetTooltip`, `SetAccelerator("CmdOrCtrl+R")`. Also `AddCheckbox`, `AddRadio`, `AddSubmenu`, `AddSectionHeader` (macOS 14+).

Tray-only mode on macOS (no Dock icon): `Mac: application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory}`. Not wanted for this app.

---

## 10. Run the frontend alone (browser) with mocks

### 10.1 Detection: inside Wails or not (VERIFIED: true in the packaged macOS app, false in Chrome)

```ts
// frontend/src/api/env.ts
// Do NOT use window._wails.environment: on macOS it is injected after the page has loaded.
export const isWails: boolean =
  typeof window !== 'undefined' &&
  (typeof (window as any).chrome?.webview?.postMessage === 'function' ||          // WebView2 (Windows)
    typeof (window as any).webkit?.messageHandlers?.external?.postMessage === 'function') // WKWebView (macOS)

// ?mock=1 forces mocks even inside Wails (demo screenshots).
export const useMocks: boolean = !isWails || new URLSearchParams(window.location.search).has('mock')
```

### 10.2 Facade: the UI imports only from `src/api` (type-checked with tsc)

```ts
// frontend/src/api/index.ts
import * as RealAccountService from '../../bindings/awsssohub/internal/app/accountservice'
import { mockAccountService } from './mocks'
import { useMocks } from './env'
export const AccountService: typeof RealAccountService = useMocks ? mockAccountService : RealAccountService
export { isWails, useMocks } from './env'
export { onEvent } from './events'
```
```ts
// frontend/src/api/mocks.ts – `typeof Real` makes tsc fail when a Go method is added/changed but the mock is not.
import { CancellablePromise } from '@wailsio/runtime'
import type * as Real from '../../bindings/awsssohub/internal/app/accountservice'
import { AccountKind } from '../../bindings/awsssohub/internal/app'
import type { Account, LoginStatus } from '../../bindings/awsssohub/internal/app'
import { mockEmit } from './events'

const ok = <T,>(v: T) => CancellablePromise.resolve(v)
export const mockAccountService: typeof Real = {
  ListAccounts: () => ok<Account[] | null>([/* fixtures */]),
  Login: (session) => {
    const st: LoginStatus = { session, loggedIn: true, kind: AccountKind.AccountKindSSO }
    return CancellablePromise.sleep(400, st).then((s) => { mockEmit('auth:status', s); return s })
  },
  // ... one entry per bound method
}
```
```ts
// frontend/src/api/events.ts
import { Events } from '@wailsio/runtime'
import { isWails } from './env'
type Handler = (data: any) => void
const local = new Map<string, Set<Handler>>()
export function onEvent<T = unknown>(name: string, cb: (data: T) => void): () => void {
  if (isWails) return Events.On(name, (ev) => cb(ev.data as T))
  const set = local.get(name) ?? new Set<Handler>(); set.add(cb); local.set(name, set)
  return () => { set.delete(cb) }
}
export function mockEmit(name: string, data: unknown) { local.get(name)?.forEach((cb) => cb(data)) }
```
Do not add `as typeof Real` to the mock object: the plain annotation `: typeof Real` is what makes tsc report a missing or changed method. VERIFIED: it type-checked in the spike.

### 10.3 Commands

```bash
cd frontend && npm run dev                       # Vite at http://127.0.0.1:9245 (strictPort); no Go needed if bindings are committed
# headless screenshot (VERIFIED):
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu \
  --user-data-dir="$(mktemp -d)" --window-size=1200,780 --virtual-time-budget=3000 \
  --screenshot="$PWD/shot.png" "http://127.0.0.1:9245/?lang=zh-CN&theme=dark"
```
Without mocks, binding calls in a browser **reject** (`POST /wails/runtime` → 404). They do not hang.

---

## 11. Theme, language, fonts

- Theme: CSS variables, driven by `prefers-color-scheme` plus an override attribute `<html data-theme="light|dark">` set from Settings. The native window chrome follows the OS (Windows `Theme: SystemDefault`; on macOS the title bar is hidden). No runtime window-theme setter exists on `WebviewWindow` in 3.1.0 (SOURCE).
- Language default: `navigator.languages.some(l => l.toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en'`. Store the user's choice in settings. The exact `navigator.language` strings in WKWebView and WebView2 are UNVERIFIED, so always use `startsWith('zh')`.
- System font stack (offline, no web fonts):
  `font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", system-ui, sans-serif;`
  Monospace: `ui-monospace, "SF Mono", Menlo, Consolas, "Cascadia Mono", monospace;`
- Window drag area on macOS (hidden-inset title bar): `InvisibleTitleBarHeight: 52` makes the top 52 px natively draggable. Keep that strip free of controls, and leave about 80 px of left padding for the traffic lights.
- Any other HTML drag region: CSS `--wails-draggable: drag` on the container. The value is inherited, so set `--wails-draggable: no-drag` on buttons and inputs inside it. SOURCE: runtime `drag.js` checks that the computed value is exactly `"drag"`; the runtime's side-effect import (`import ... from '@wailsio/runtime'`) loads it.

---

## 12. Running `aws` / `git` from the app (`internal/execx`; compiled for darwin + windows, unit-tested on darwin)

```go
// Package execx runs external tools (aws, git) with a sane PATH and no console windows.
package execx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	pathOnce sync.Once
	fullPath string
)

// EnvPATH: preferred install dirs, login-shell PATH (macOS), inherited PATH, system dirs.
func EnvPATH() string {
	pathOnce.Do(func() {
		parts := preferredDirs()
		if runtime.GOOS == "darwin" {
			parts = append(parts, loginShellPATH()...)
		}
		parts = append(parts, filepath.SplitList(os.Getenv("PATH"))...)
		parts = append(parts, systemDirs()...)
		fullPath = strings.Join(dedupe(parts), string(os.PathListSeparator))
	})
	return fullPath
}

func LookPath(name string) (string, error) {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		name += ".exe"
	}
	for _, dir := range filepath.SplitList(EnvPATH()) {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	bin := name
	if p, err := LookPath(name); err == nil {
		bin = p
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	env := withEnv(os.Environ(), "PATH", EnvPATH())
	env = withEnv(env, "AWS_PAGER", "")
	env = withEnv(env, "GIT_TERMINAL_PROMPT", "0")
	env = withEnv(env, "GCM_INTERACTIVE", "never")
	cmd.Env = env
	hideWindow(cmd)
	return cmd
}

func loginShellPATH() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const marker = "__AWSSSOHUB_PATH__"
	// -l only (fast, ~0.05s). -i -l is ~2s and may print; use it only as a fallback if aws is not found.
	out, err := exec.CommandContext(ctx, shell, "-l", "-c", `printf '`+marker+`%s`+marker+`' "$PATH"`).Output()
	if err != nil {
		return nil
	}
	s := string(out)
	i, j := strings.Index(s, marker), strings.LastIndex(s, marker)
	if i < 0 || j <= i {
		return nil
	}
	return filepath.SplitList(s[i+len(marker) : j])
}

func preferredDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin")}
	case "windows":
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		return []string{
			filepath.Join(pf, "Amazon", "AWSCLIV2"),
			filepath.Join(pf, "Git", "cmd"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git", "cmd"),
			filepath.Join(home, "scoop", "shims"),
		}
	default:
		return []string{"/usr/local/bin"}
	}
}

func systemDirs() []string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return []string{filepath.Join(root, "System32"), root}
	}
	return []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(p)
		}
		if p == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func withEnv(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, key) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, key+"="+val)
}
```
```go
//go:build windows

package execx

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000 // CREATE_NO_WINDOW
// If git's credential-helper child (aws.exe) flashes a console on Windows, use for git:
// CreationFlags: 0x00000010 /* CREATE_NEW_CONSOLE */ with HideWindow: true (hidden console inherited by children). UNVERIFIED.

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
```
```go
//go:build !windows

package execx

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
```
Test result with `PATH=/usr/bin:/bin:/usr/sbin:/sbin`: `aws -> /opt/homebrew/bin/aws`, `git -> /opt/homebrew/bin/git` (2.56.0). Unit tests must not depend on these exact paths. Inject the dir lists for tests.

macOS: before you use `/usr/bin/git`, check that `xcode-select -p` succeeds. Otherwise the shim pops up the CLT installer.

---

## 13. App identity files and icons

| What | File(s) |
|---|---|
| Binary / `.app` / `.exe` file name | `Taskfile.yml` → `vars: APP_NAME: "aws-sso-hub"` |
| macOS display name, bundle id, version | `build/darwin/Info.plist`: `CFBundleName` = `AWS SSO Hub`, `CFBundleIdentifier`, `CFBundleShortVersionString` + `CFBundleVersion`, `LSMinimumSystemVersion` = `13.0.0`, `NSHumanReadableCopyright`. Same keys in `build/darwin/Info.dev.plist` (used by `wails3 dev`) |
| Windows version resource | `build/windows/info.json` (`fixed.file_version`, `fixed.product_version`, `info."0409".ProductName/CompanyName/FileDescription/LegalCopyright`) |
| Windows manifest | `build/windows/wails.exe.manifest` (`assemblyIdentity name` = bundle id, `version` = 4-part or 3-part version; DPI awareness `permonitorv2`; `asInvoker`) |
| NSIS metadata | `build/windows/nsis/wails_tools.nsh` (`INFO_PROJECTNAME`, `INFO_COMPANYNAME`, `INFO_PRODUCTNAME`, `INFO_PRODUCTVERSION`, `INFO_COPYRIGHT`) |
| Template data (only used by `update build-assets`, which we do not run) | `build/config.yml` `info:`. Keep it in sync for documentation only |
| In-app version | Go const, e.g. `internal/version.Version = "0.1.0"`, exposed via a bound `GetAppInfo()`. Bump all of the above together (write a `scripts/set-version.sh`) |

Icons: there is one source, `build/appicon.png` (1024×1024 PNG with alpha). After the §2.3 edits:

```bash
# From an SVG (macOS built-in sips; VERIFIED 1024x1024 with alpha). /opt/homebrew/bin/rsvg-convert also works.
sips -s format png design/appicon.svg --out build/appicon.png
# Regenerate (also runs automatically in every build via common:generate:icons):
wails3 generate icons -input build/appicon.png -macfilename build/darwin/icons.icns -windowsfilename build/windows/icon.ico
```
Outputs: `build/darwin/icons.icns` (16 to 1024 px) and `build/windows/icon.ico` (256, 128, 64, 48, 32, 16). The tray icon PNGs are separate files embedded with `//go:embed`.

---

## 14. Build and package commands (VERIFIED on macOS 27.2 arm64)

**Always `rm -rf bin` before each target.** A leftover universal binary makes later thin builds fail, or silently skip writing (wails.md E2/E3).

```bash
# macOS arm64 .app            -> bin/aws-sso-hub.app           (~7 s warm, 11 MB)
rm -rf bin && wails3 package
# macOS amd64 .app            -> bin/aws-sso-hub.app (x86_64)  (~8 s warm, 10 MB)
rm -rf bin && wails3 package GOARCH=amd64
# macOS universal .app        -> bin/aws-sso-hub.app (x86_64+arm64) (6 s warm … 101 s cold, 20 MB)
rm -rf bin && wails3 task darwin:package:universal
# Windows amd64 exe           -> bin/aws-sso-hub.exe (PE32+ GUI x86-64) (3 s warm … 46 s cold, 11.5 MB)
rm -rf bin && wails3 build GOOS=windows GOARCH=amd64
#   equivalent: wails3 task windows:build ARCH=amd64
#   WRONG:      wails3 build GOOS=windows ARCH=amd64   (ARCH= is overridden → arm64 exe)
```

Release script outline (`scripts/release.sh`, to be written):
```bash
set -euo pipefail
VERSION=0.1.0; OUT=dist; mkdir -p "$OUT"
rm -rf bin && wails3 task darwin:package:universal
lipo -info bin/aws-sso-hub.app/Contents/MacOS/aws-sso-hub | grep -q 'x86_64 arm64'
codesign --verify --deep --strict bin/aws-sso-hub.app
rm -rf "$OUT/AWS SSO Hub.app" && ditto bin/aws-sso-hub.app "$OUT/AWS SSO Hub.app"
(cd "$OUT" && ditto -c -k --keepParent "AWS SSO Hub.app" "AWS-SSO-Hub-$VERSION-macos-universal.zip")
rm -rf bin && wails3 build GOOS=windows GOARCH=amd64
file bin/aws-sso-hub.exe | grep -q 'x86-64'
cp bin/aws-sso-hub.exe "$OUT/AWS-SSO-Hub-$VERSION-windows-amd64.exe"
(cd "$OUT" && zip -q "AWS-SSO-Hub-$VERSION-windows-amd64.zip" "AWS-SSO-Hub-$VERSION-windows-amd64.exe")
```

Optional / later:
- DMG: `wails3 task darwin:package:dmg` (UNVERIFIED in the spike).
- NSIS installer: needs `makensis` (**not installed; do not install without the user's OK**: `brew install makensis`). Then `rm -rf bin && wails3 package GOOS=windows GOARCH=amd64 INSTALL_SCOPE=user` gives `bin/aws-sso-hub-amd64-installer.exe` (per-user install to `%LOCALAPPDATA%\Programs\AWS SSO Hub`, bundles the WebView2 bootstrapper). Without makensis this fails with `"makensis": executable file not found in $PATH`. VERIFIED that it fails today.
- Signing later: `wails3 setup` (signing wizard), `wails3 tool sign --input bin/aws-sso-hub.app --notarize`, and `wails3 tool sign --input bin/aws-sso-hub.exe --certificate ...`.

Requirements: Xcode Command Line Tools (CGO for macOS), Go ≥ 1.26, npm. **No Docker.** Windows builds use `CGO_ENABLED=0`.

---

## 15. Dev mode

```bash
wails3 dev        # = Taskfile `dev`: wails3 dev -config ./build/config.yml -port 9245
```
- First start took about 38 s. It builds the Go app with `DEV=true` (no `-tags production`, so debug logs are on), starts Vite on `127.0.0.1:9245`, wraps the app in `bin/aws-sso-hub.dev.app` (uses `Info.dev.plist`) and connects the app to the Vite server. Frontend changes hot-reload. Go changes (`*.go`, debounce 1000 ms) trigger a rebuild, binding regeneration and a restart.
- Port override: env `WAILS_VITE_PORT`.
- Dev and production builds use the same `SingleInstance.UniqueID`, so you cannot run the installed app and `wails3 dev` at the same time. Quit one first.
- Production builds have no console log output. Use a file logger (`Options.Logger`), with secrets redacted, for diagnostics.
- WebView devtools: `DevToolsEnabled` / `OpenInspectorOnStartup` on the window options (dev only; UNVERIFIED default in production).

---

## 16. Quick reference: things that bit us

| Do | Don't |
|---|---|
| `wails3 build GOOS=windows GOARCH=amd64` | `... ARCH=amd64` with `wails3 build` |
| `rm -rf bin` before every target | Reuse `bin/` across arches |
| `import { Call } ...; e instanceof Call.RuntimeError` | `import { RuntimeError }` (not exported at top level) |
| tsconfig `ES2022` | ES2020 (no `Error.cause`) |
| Return `make([]T, 0)` from Go | Return nil slices (they become `null`) |
| Keep AWS account IDs as `string` | int64 (becomes a JS number) |
| Emit exactly the registered type | Emit `*T` when `T` was registered |
| `menu.Update()` after changing a tray menu | Change the items and expect a refresh |
| Detect Wails via `chrome.webview` / `webkit.messageHandlers.external` | `window._wails.environment` (injected late on macOS) |
| Route links through `Browser.OpenURL` | Plain `<a target=_blank>` |
| Commit `frontend/bindings` | Gitignore it (breaks frontend-only builds) |

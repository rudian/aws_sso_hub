package app

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/version"
)

// ---- pure menu model (unit tested) -------------------------------------------------------------------------

type trayKind int

const (
	trayDisabled trayKind = iota // informational line
	traySeparator
	trayLogin // sign in to Arg
	trayOpen
	trayQuit
)

type trayItem struct {
	Kind  trayKind
	Label string
	Arg   string
}

func zh(lang string) bool { return lang != "en" }

func pick(lang, zhText, enText string) string {
	if zh(lang) {
		return zhText
	}
	return enText
}

// remainingLabel renders the session time left as `6:12` (>= 1 h) or `48m`; "" when it is unknown.
func remainingLabel(s api.SessionStatus, now time.Time) string {
	if !s.SessionKnown || s.SessionOverrun || s.SessionEndsAt == "" {
		return ""
	}
	end, err := time.Parse(time.RFC3339, s.SessionEndsAt)
	if err != nil {
		return ""
	}
	left := end.Sub(now)
	if left <= 0 {
		return ""
	}
	if left < time.Hour {
		return fmt.Sprintf("%dm", int(left.Minutes()))
	}
	return fmt.Sprintf("%d:%02d", int(left.Hours()), int(left.Minutes())%60)
}

func portalStateLabel(s api.SessionStatus, lang string, now time.Time) string {
	switch s.State {
	case api.SessionActive, api.SessionExpiring, api.SessionRefreshable:
		l := pick(lang, "已登录", "Signed in")
		if rem := remainingLabel(s, now); rem != "" {
			l += " " + rem
		}
		return l
	case api.SessionExpired, api.SessionError:
		return pick(lang, "已过期", "Expired")
	default: // signed-out, signing-in
		return pick(lang, "未登录", "Not signed in")
	}
}

// buildTrayModel derives the tray menu from the accounts state.
func buildTrayModel(st api.AccountsState, lang string, now time.Time) []trayItem {
	items := []trayItem{{Kind: trayDisabled, Label: version.ProductName}}
	var needLogin []string
	for _, p := range st.Portals {
		if p.Draft || p.LegacyFormat {
			continue
		}
		items = append(items, trayItem{Kind: trayDisabled, Label: p.Name + "  " + portalStateLabel(p.Session, lang, now)})
		for _, prof := range p.Profiles {
			items = append(items, trayItem{Kind: trayDisabled, Label: "  " + prof})
		}
		if p.Session.State != api.SessionActive {
			needLogin = append(needLogin, p.Name)
		}
	}
	for _, pr := range st.Profiles {
		if pr.Kind == api.ProfileKindStatic {
			items = append(items, trayItem{Kind: trayDisabled, Label: pr.Name + "  " + pick(lang, "访问密钥（待迁移）", "Access key (to migrate)")})
		}
	}
	items = append(items, trayItem{Kind: traySeparator})
	for _, name := range needLogin {
		items = append(items, trayItem{Kind: trayLogin, Arg: name, Label: pick(lang, "登录 "+name+"…", "Sign in to "+name+"…")})
	}
	items = append(items,
		trayItem{Kind: trayOpen, Label: pick(lang, "打开 "+version.ProductName, "Open "+version.ProductName)},
		trayItem{Kind: trayQuit, Label: pick(lang, "退出", "Quit")})
	return items
}

// ---- Wails tray ----------------------------------------------------------------------------------------------

// trayController owns the system tray icon. It exists only while the tray setting is on.
type trayController struct {
	sh *Shell

	mu      sync.Mutex
	tray    *application.SystemTray
	menu    *application.Menu
	dirty   chan struct{}
	stop    chan struct{}
	running bool
}

func newTrayController(sh *Shell) *trayController {
	return &trayController{sh: sh}
}

// Enable creates the tray (idempotent).
func (t *trayController) Enable() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	app := application.Get()
	if app == nil {
		t.mu.Unlock()
		return
	}
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIconPNG(true))
	} else {
		tray.SetIcon(trayIconPNG(false))
	}
	tray.SetTooltip(version.ProductName)
	t.tray = tray
	t.menu = app.NewMenu()
	tray.SetMenu(t.menu)
	t.dirty = make(chan struct{}, 1)
	t.stop = make(chan struct{})
	t.running = true
	dirty, stop := t.dirty, t.stop
	t.mu.Unlock()

	t.Refresh()
	go func() {
		tick := time.NewTicker(time.Minute) // countdown labels change without any event
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-dirty:
				t.rebuild()
			case <-tick.C:
				t.rebuild()
			}
		}
	}()
}

// Disable removes the tray (idempotent).
func (t *trayController) Disable() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	close(t.stop)
	tray := t.tray
	t.tray, t.menu = nil, nil
	t.mu.Unlock()
	if tray != nil {
		tray.Destroy()
	}
}

// Refresh asks for a menu rebuild (coalesced).
func (t *trayController) Refresh() {
	t.mu.Lock()
	ch := t.dirty
	t.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (t *trayController) rebuild() {
	t.mu.Lock()
	menu := t.menu
	t.mu.Unlock()
	if menu == nil {
		return
	}
	c := t.sh.c
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := c.Accounts.State(ctx)
	if err != nil {
		return
	}
	items := buildTrayModel(st, c.Settings.UILanguage(), c.now())
	menu.Clear()
	for _, it := range items {
		it := it
		switch it.Kind {
		case traySeparator:
			menu.AddSeparator()
		case trayDisabled:
			menu.Add(it.Label).SetEnabled(false)
		case trayLogin:
			menu.Add(it.Label).OnClick(func(*application.Context) {
				t.sh.showWindow()
				c.emit(api.EventNavigate, api.NavigateRequest{Page: "accounts", Action: "login", Arg: it.Arg})
			})
		case trayOpen:
			menu.Add(it.Label).OnClick(func(*application.Context) { t.sh.showWindow() })
		case trayQuit:
			menu.Add(it.Label).OnClick(func(*application.Context) {
				if a := application.Get(); a != nil {
					a.Quit()
				}
			})
		}
	}
	menu.Update()
}

func labelsOf(items []trayItem) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Label)
		b.WriteByte('\n')
	}
	return b.String()
}

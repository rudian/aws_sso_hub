// Command testapp is the S0-02 stand-in for Codepass: a headless program that
// lives in a .app bundle and updates itself through internal/update.
//
//	testapp          launch mode (what `open -n` starts after a swap): record and exit
//	testapp update   check, download, gate, hand over to the Wails helper
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/rudian/codepass/internal/update"
)

// Set by -ldflags for each build.
var (
	version  = "0.0.0"
	workRoot = ""
)

type config struct {
	Server      string   `json:"server"`
	Repo        string   `json:"repo"`
	Keys        []string `json:"keys"` // hex ed25519 public keys: current, next
	BundleID    string   `json:"bundleId"`
	Requirement string   `json:"requirement"`
	Gatekeeper  bool     `json:"gatekeeper"`
}

type host struct{ quit chan struct{} }

func (h *host) Emit(string, ...any) bool                              { return true }
func (h *host) OnEvent(string, func(any)) func()                      { return func() {} }
func (h *host) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (h *host) Quit()                                                 { close(h.quit) }

func main() {
	updater.HandleHelperMode() // MUST stay the first statement of main (plan 4.14 item 6)

	hi := update.Highest{Path: filepath.Join(workRoot, "state", "highest")}
	if err := hi.Record(version); err != nil {
		fmt.Println("REFUSED:", err)
		os.Exit(3)
	}

	if len(os.Args) < 2 {
		f, _ := os.OpenFile(filepath.Join(workRoot, "marker", "launches.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		fmt.Fprintf(f, "started %s pid=%d\n", version, os.Getpid())
		f.Close()
		return
	}

	var cfg config
	b, err := os.ReadFile(filepath.Join(workRoot, "config.json"))
	check(err)
	check(json.Unmarshal(b, &cfg))
	keys, err := update.ParseKeys(cfg.Keys)
	check(err)
	h := &host{quit: make(chan struct{})}
	ch := update.Channel{Name: "test", Repository: cfg.Repo, Prerelease: true, Keys: keys, BaseURL: cfg.Server}
	c := update.NewController(update.ControllerConfig{
		Quit:         h.Quit,
		Running:      version,
		Gate:         update.Gate{BundleID: cfg.BundleID, Requirement: cfg.Requirement, RequireGatekeeper: cfg.Gatekeeper},
		Highest:      hi,
		HelperLog:    filepath.Join(workRoot, "state", "helperlog"),
		StagedRecord: filepath.Join(workRoot, "state", "staged"),
		Channel:      func() (update.Channel, error) { return ch, nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if os.Args[1] == "ungated" {
		// Control experiment: the Wails updater alone (signed-manifest provider
		// but NO bundle gate) to show what its own macOS check lets through.
		prov, err := update.NewProvider(update.ProviderConfig{Channel: ch, Running: version, Highest: hi.Get})
		check(err)
		u := updater.New(h)
		check(u.Init(updater.Config{CurrentVersion: version, Providers: []updater.Provider{prov}, Window: updater.WindowNone}))
		rel, err := u.Check(ctx)
		check(err)
		if rel == nil {
			fmt.Println("UP-TO-DATE")
			os.Exit(4)
		}
		check(u.DownloadAndInstall(ctx))
		check(u.Restart(ctx))
		<-h.quit
		fmt.Println("QUIT for helper (ungated)")
		return
	}
	st, err := c.Check(ctx)
	if errors.Is(err, update.ErrUpToDate) {
		fmt.Println("UP-TO-DATE")
		os.Exit(4)
	}
	if err != nil {
		fmt.Println("REFUSED:", err)
		os.Exit(3)
	}
	if err := c.Download(ctx); err != nil {
		fmt.Println("REFUSED:", err)
		os.Exit(3)
	}
	fmt.Printf("STAGED %s notes=%q\n", st.Version, st.Notes)
	if err := c.Restart(ctx); err != nil {
		fmt.Println("REFUSED:", err)
		os.Exit(3)
	}
	select {
	case <-h.quit:
		fmt.Println("QUIT for helper")
	case <-time.After(10 * time.Second):
		fmt.Println("host.Quit never called")
		os.Exit(5)
	}
}

func check(err error) {
	if err != nil {
		fmt.Println("REFUSED:", err)
		os.Exit(3)
	}
}

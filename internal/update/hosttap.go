package update

import (
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// hostTap is the updater.Host given to Wails. It reads download progress from Wails' events and passes Quit to the
// app; the update window is never opened.
type hostTap struct {
	quit       func()
	onProgress func(written, total int64)
}

func (h *hostTap) Emit(name string, data ...any) bool {
	if name == updater.EventDownloadProgress && len(data) == 1 && h.onProgress != nil {
		if p, ok := data[0].(updater.Progress); ok {
			h.onProgress(p.Written, p.Total)
		}
	}
	return true
}
func (h *hostTap) OnEvent(string, func(any)) func()                      { return func() {} }
func (h *hostTap) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (h *hostTap) Quit() {
	if h.quit != nil {
		h.quit()
	}
}

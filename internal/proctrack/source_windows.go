//go:build windows

package proctrack

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winSource asks about the PIDs the caller wants (the members of the sessions' jobs and what is already
// tracked), never the whole table. Every descendant of a session stays in its job, so no parent links are
// needed: PPID is left 0 and the tracker's job members are the whole set. CPU is the change in processor time
// between two reads.
type winSource struct {
	mu   sync.Mutex
	prev map[int]cpuSample
}

type cpuSample struct {
	cpu time.Duration
	at  time.Time
}

// NewSource is the process-table source of this platform.
func NewSource() Source { return &winSource{prev: map[int]cpuSample{}} }

type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var (
	psapi                = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemory = psapi.NewProc("GetProcessMemoryInfo")
)

func (w *winSource) Read(want []int) ([]Proc, error) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	next := make(map[int]cpuSample, len(want))
	var out []Proc
	for _, pid := range want {
		if pid <= 0 {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			continue // gone, or not ours to ask about
		}
		p := Proc{PID: pid}
		var creation, exit, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
			windows.CloseHandle(h)
			continue
		}
		if exit.HighDateTime != 0 || exit.LowDateTime != 0 { // already exited, handle still open elsewhere
			windows.CloseHandle(h)
			continue
		}
		p.Start = creation.Nanoseconds() / 1e9
		cpu := time.Duration(int64(kernel.HighDateTime)<<32|int64(kernel.LowDateTime))*100 +
			time.Duration(int64(user.HighDateTime)<<32|int64(user.LowDateTime))*100
		next[pid] = cpuSample{cpu: cpu, at: now}
		if old, ok := w.prev[pid]; ok && now.After(old.at) && cpu >= old.cpu {
			p.CPU = float64(cpu-old.cpu) / float64(now.Sub(old.at)) * 100
		}
		var buf [windows.MAX_LONG_PATH]uint16
		n := uint32(len(buf))
		if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err == nil {
			p.Comm = windows.UTF16ToString(buf[:n])
		}
		var mc processMemoryCounters
		mc.cb = uint32(unsafe.Sizeof(mc))
		if r, _, _ := procGetProcessMemory.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.cb)); r != 0 {
			p.RSS = int64(mc.WorkingSetSize)
		}
		windows.CloseHandle(h)
		out = append(out, p)
	}
	w.prev = next
	return out, nil
}

// defaultCommands: Windows keeps no command line in the source; the UI shows the program's name only.
func defaultCommands([]int) ([]CommandLine, error) { return nil, nil }

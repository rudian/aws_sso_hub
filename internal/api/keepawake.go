package api

// Keep-awake (plan 4.10, A2-03). internal/keepawake holds the operating system's "stay awake" assertions while AI
// work is going on; these are the types the window sees.

// EventKeepAwakeChanged carries a KeepAwakeState whenever the hold, the counts or the settings change.
const EventKeepAwakeChanged = "keepawake:changed"

// Values of KeepAwakeState.Error.
const (
	KeepAwakeErrorPartial = "partial"
	KeepAwakeErrorNone    = "none"
)

// KeepAwakeConfig is the Keep awake card of Settings (design 5.14).
type KeepAwakeConfig struct {
	// Enabled: "AI 任务运行时不让电脑睡眠". Turning it off releases the hold at once, even with tasks running.
	Enabled bool `json:"enabled"`
	// ScreenToo: "屏幕也保持常亮". Off uses the idle-sleep assertion only, so the screen can lock.
	ScreenToo bool `json:"screenToo"`
	// GraceMinutes: how long a task that waits for the owner keeps the hold: 5 | 15 | 60, or 0 = as long as the
	// ceiling allows ("一直").
	GraceMinutes int `json:"graceMinutes"`
	// CeilingHours: the hard ceiling, 4 | 8 | 12. After this long without a hook event or input from the owner,
	// the hold is released even if a task still looks busy.
	CeilingHours int `json:"ceilingHours"`
}

// KeepAwakeState is what the chips, the AI tasks banner and the Settings badge show.
type KeepAwakeState struct {
	// Held is true exactly while at least one sleep assertion of the operating system is held by this app (the App
	// Nap activity does not count: it does not stop the computer from sleeping).
	Held bool `json:"held"`
	// Active is the number of tasks and closed sessions that count as activity right now (Running + Waiting +
	// Leftover). It is non-zero while the setting is off too: the chip then shows nothing, the card shows the count.
	Active int `json:"active"`
	// Running: AI tasks that are working. Waiting: AI tasks waiting for the owner inside the grace time.
	// Leftover: closed sessions whose programs are still running (plan 4.8b).
	Running  int `json:"running"`
	Waiting  int `json:"waiting"`
	Leftover int `json:"leftover"`
	// Programs is the number of programs those closed sessions (Leftover) still run, inside their deadline: the
	// "2 个后台程序" of the sidebar chip (design 5.6a).
	Programs int `json:"programs"`
	// Screen is true while an assertion that keeps the display on is really held (never just because the setting is on).
	Screen bool `json:"screen"`
	// ReleaseAt is the time (unix milliseconds) at which the hold ends if nothing changes. It is set only while
	// nothing is running and something waits or is left behind (every one of those has a deadline); a running task
	// renews its own ceiling with every event, so it has no end time to show. 0 = none.
	ReleaseAt int64 `json:"releaseAt"`
	// CeilingHit is true when a task was dropped because of the hard ceiling and nothing has revived it.
	CeilingHit bool `json:"ceilingHit"`
	// Error is a stable code, never text: "" = none, KeepAwakeErrorPartial = some assertions could not be taken
	// while another is held, KeepAwakeErrorNone = none could be taken. The window words it per platform; the holder
	// names and details go to the log only.
	Error  string          `json:"error"`
	Config KeepAwakeConfig `json:"config"`
}

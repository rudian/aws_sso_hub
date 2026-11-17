package api

// AI tasks (plan 4.7, 4.8, 4.8b; A2-08). internal/tasks owns the lifecycle; these are the types the window sees.

const (
	// EventTasksChanged carries a TasksState whenever a task, its state or the programs a closed session left
	// running changed. It is coalesced: one event may stand for several changes.
	EventTasksChanged = "tasks:changed"
	// EventTaskOpen carries a TaskOpen: the person pressed a notification about a task (the window is already
	// shown); the page opens that task's terminal.
	EventTaskOpen = "tasks:open"
)

// TaskState is what the task list shows (plan 4.8). The values are stable strings, never text.
type TaskState string

const (
	TaskStarting TaskState = "starting"
	TaskRunning  TaskState = "running"
	TaskWaiting  TaskState = "waiting"
	// TaskNoSignal is "running (no signal)": the tool is alive but sends no events.
	TaskNoSignal TaskState = "running-no-signal"
	TaskDone     TaskState = "done"
	TaskFailed   TaskState = "failed"
	TaskStopped  TaskState = "stopped"
	// TaskClosedRunning: the tool exited or the terminal was closed while programs it started are still alive.
	TaskClosedRunning TaskState = "closed-running"
)

// TaskWait says why a waiting task waits (the tracker's reasons).
type TaskWait string

const (
	TaskWaitNone       TaskWait = ""
	TaskWaitPermission TaskWait = "permission"
	TaskWaitQuestion   TaskWait = "question"
	TaskWaitIdle       TaskWait = "idle"
	TaskWaitTurnEnd    TaskWait = "turn_end"
	TaskWaitLimit      TaskWait = "limit"
	TaskWaitError      TaskWait = "error"
)

// TaskNameSource says where a task's title came from.
type TaskNameSource string

const (
	TaskNamedByAI     TaskNameSource = "ai"
	TaskNamedFallback TaskNameSource = "fallback" // the quiet fallback of plan 4.7
	TaskNamedPending  TaskNameSource = "pending"  // "New task <n>" until the first message names it
	TaskNamedByPerson TaskNameSource = "manual"   // renamed by the person
)

// TaskWhere is where a task runs. Only "main" exists until milestone 3 adds the own workspace.
type TaskWhere string

const (
	TaskWhereMain TaskWhere = "main"
	TaskWhereOwn  TaskWhere = "own"
)

// TaskLeftovers is what the process tracker reports for the task's session (cached; never a command line).
type TaskLeftovers struct {
	Count  int     `json:"count"`
	Others int     `json:"others"` // programs the tool started: Count minus the shell and the tool (only the tool when the shell exec'd it)
	CPU    float64 `json:"cpu"`
	Memory int64   `json:"memory"`
	// ClosedAt is when the session was closed (RFC 3339), "" while it lives.
	ClosedAt string `json:"closedAt,omitempty"`
}

// Task is one AI task.
type Task struct {
	ID             string         `json:"id"`   // also the terminal session ID and the name of <data>/sessions/<id>
	Repo           string         `json:"repo"` // the repository's stable ID (F1-18)
	Tool           AIToolID       `json:"tool"`
	Title          string         `json:"title"`
	Slug           string         `json:"slug"`
	NamedBy        TaskNameSource `json:"namedBy"`
	Number         int            `json:"number"` // the n of "New task <n>"
	Where          TaskWhere      `json:"where"`
	Dir            string         `json:"dir"`
	State          TaskState      `json:"state"`
	Reason         TaskWait       `json:"reason"`
	CreatedAt      string         `json:"createdAt"`                // RFC 3339
	StartedAt      string         `json:"startedAt"`                // the latest start or resume
	EndedAt        string         `json:"endedAt,omitempty"`        // empty while the tool runs
	ExitCode       *int           `json:"exitCode,omitempty"`       // set when the tool exited by itself
	Live           bool           `json:"live"`                     // the tool runs now
	Resumable      bool           `json:"resumable"`                // an ended task whose conversation can be reopened
	Resumed        bool           `json:"resumed"`                  // the tool said its last start really resumed the conversation
	PermissionMode string         `json:"permissionMode,omitempty"` // as of the last event
	Leftovers      TaskLeftovers  `json:"leftovers"`
}

// TasksState is the whole list with the counts the sidebar, the rail and the update dialog read.
type TasksState struct {
	Tasks []Task `json:"tasks"`
	// Running counts tasks whose state is starting, running or running (no signal): what "Reopen when no task is
	// running" waits for. Waiting counts tasks that need the person. Live counts tasks whose tool runs.
	Running int `json:"running"`
	Waiting int `json:"waiting"`
	Live    int `json:"live"`
	// Closed counts tasks in the state closed-running; Programs the programs they still run.
	Closed   int `json:"closed"`
	Programs int `json:"programs"`
	// Cap is the soft cap: starting a task while Live >= Cap asks first.
	Cap int `json:"cap"`
}

// CreateTaskRequest is the new-task sheet's Start.
type CreateTaskRequest struct {
	Repo        string    `json:"repo"` // the repository's stable ID
	Tool        AIToolID  `json:"tool"`
	Where       TaskWhere `json:"where"`
	Description string    `json:"description"`
	// Confirmed says the person accepted the soft-cap question ("N tasks are already running").
	Confirmed bool `json:"confirmed"`
}

// Parameters of the E_INVALID_INPUT error that asks the soft-cap question (field "cap"): "running" and "cap".
const TaskFieldCap = "cap"

// TaskOpen is the payload of EventTaskOpen.
type TaskOpen struct {
	Task string `json:"task"`
	Repo string `json:"repo"`
}

// LiveSession is one row of the quit dialog (design 5.6b).
type LiveSession struct {
	Task     string    `json:"task"`
	Repo     string    `json:"repo"`
	RepoName string    `json:"repoName"` // the repository's local folder name
	Tool     AIToolID  `json:"tool"`
	Title    string    `json:"title"`
	State    TaskState `json:"state"`
	// Programs is the number of programs the tool started that are alive ("started 2 programs").
	Programs int `json:"programs"`
}

// LiveSummary feeds the quit dialog: the live sessions and the closed ones that still run programs.
type LiveSummary struct {
	Sessions []LiveSession `json:"sessions"`
	Live     int           `json:"live"`   // sessions whose tool runs
	Closed   int           `json:"closed"` // closed sessions that left programs
	Programs int           `json:"programs"`
}

// StopAllResult is what "Stop all and quit" did.
type StopAllResult struct {
	Stopped int `json:"stopped"` // tasks stopped
	// Survivors is the number of programs that could not be stopped (another user's, or restarted by the
	// system); the next start lists them as closed, still running.
	Survivors int    `json:"survivors"`
	Error     string `json:"error,omitempty"` // set when the stop was not clean
}

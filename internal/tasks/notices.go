package tasks

import (
	"fmt"
	"strings"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/notify"
)

// The two notifications this package sends (design 6). Only the words are ours; the banner is the system's.
// Neither holds a path, a command, the task's description or anything the AI said.
const (
	// KindTaskWaiting: a task changed to "waiting" while the window was not in front. Default sound. Click shows
	// the window and opens that task's terminal. One notification per task: a second one replaces the first.
	KindTaskWaiting notify.Kind = "task.waiting"
	// KindStillRunning: the first time closing the window hid it while tasks ran. Once ever. No sound. Click
	// shows the window.
	KindStillRunning notify.Kind = "app.stillrunning"
)

func isZH(lang string) bool { return strings.HasPrefix(lang, "zh") }

// WaitingNotice is "AI task waiting": the title is the repository's local folder name, the body names the tool.
func WaitingNotice(lang, taskID, repoID, repoName string, tool aitools.ID) notify.Notice {
	body := fmt.Sprintf("%s is waiting for your answer", tool.Name())
	if isZH(lang) {
		body = fmt.Sprintf("%s 在等你回答", tool.Name())
	}
	return notify.Notice{
		Kind: KindTaskWaiting, ID: waitingID(taskID), Title: repoName, Body: body,
		Data: map[string]string{"task": taskID, "repo": repoID},
		Tone: api.NotifyToneWarn,
	}
}

func waitingID(taskID string) string { return "task-waiting:" + taskID }

// StillRunningNotice is "Codepass is still running" with the number of tasks that were not interrupted.
func StillRunningNotice(lang string, n int) notify.Notice {
	title, body := "Codepass is still running", fmt.Sprintf("%d AI tasks were not interrupted", n)
	if n == 1 {
		body = "1 AI task was not interrupted"
	}
	if isZH(lang) {
		title, body = "Codepass 还在运行", fmt.Sprintf("%d 个 AI 任务没有中断", n)
	}
	return notify.Notice{Kind: KindStillRunning, ID: "app.stillrunning", Title: title, Body: body, Silent: true, Tone: api.NotifyToneInfo}
}

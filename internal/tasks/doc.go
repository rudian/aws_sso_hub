// Package tasks is the AI task lifecycle (plan 4.7, 4.8, 4.8a, 4.8b, 4.10; A2-08): it creates, starts, stops and
// resumes tasks and joins the pieces that group B built, each of which stays ignorant of the others:
//
//   - internal/terminal runs the tool in a terminal session named by the task ID;
//   - internal/aitools builds the launch (login shell, arguments, session ID, account variable);
//   - internal/taskstate says what the tool is doing (hooks, fallback);
//   - internal/proctrack follows the programs the session started, also after it closed;
//   - internal/keepawake holds the computer awake while that matters;
//   - internal/notify tells the person that a task waits and that the app still runs.
//
// The records live in <data>/tasks.json; everything one task owns on disk lives in <data>/sessions/<task-id>/
// (the hook plugin, the saved scrollback, later the handoff notes). Nothing here logs terminal bytes, hook
// payloads or the task description.
//
// Threads. Bound methods run on Wails' own goroutines, never on the UI thread, and may take seconds (naming,
// stopping). The window-close hook runs on the UI thread and reads TasksRunning, which is a cached flag. The
// tracker's onChange and proctrack's OnChange must be quick: OnState and ProcChanged only update memory and
// hand the rest (events, notifications, files) to other goroutines.
package tasks

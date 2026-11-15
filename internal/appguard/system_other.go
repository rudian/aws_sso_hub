//go:build !darwin && !windows

package appguard

import "errors"

func trashDir() string { return "" }

const canRecycle = false

func recycleFile(string) error { return errors.New("no Recycle Bin on this platform") }

type sysPrompter struct{}

func newSysPrompter() Prompter { return sysPrompter{} }

func (sysPrompter) AskQuit([]Instance, string) bool { return false }
func (sysPrompter) AskTrash([]string, string) bool  { return false }
func (sysPrompter) Tell(string)                     {}

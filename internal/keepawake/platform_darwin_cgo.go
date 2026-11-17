//go:build darwin && cgo

package keepawake

func platformActivityAPI() ActivityAPI { return NewActivityAPI() }

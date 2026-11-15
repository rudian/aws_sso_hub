//go:build !darwin && !windows

package update

func platformBackend(service, account string) secretBackend { return nil }

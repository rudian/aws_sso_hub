//go:build windows

package browser

import (
	"golang.org/x/sys/windows/registry"
)

// winDefaultCommand reads the shell\open\command of the https handler chosen by the user. It reads the
// registry directly, so localized reg.exe output and non-ASCII paths are no problem.
func winDefaultCommand() (string, error) {
	progID, err := pickProgID(func(key string) (string, error) {
		k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
		if err != nil {
			return "", err
		}
		defer k.Close()
		v, _, err := k.GetStringValue("ProgId")
		return v, err
	})
	if err != nil {
		return "", err
	}
	c, err := registry.OpenKey(registry.CLASSES_ROOT, progID+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer c.Close()
	val, typ, err := c.GetStringValue("")
	if err != nil {
		return "", err
	}
	if typ == registry.EXPAND_SZ {
		if val, err = registry.ExpandString(val); err != nil {
			return "", err
		}
	}
	return val, nil
}

// winPolicy reads a policy DWORD, machine policy first, then user policy.
func winPolicy(subkey, name string) (uint32, bool) {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		k, err := registry.OpenKey(root, subkey, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetIntegerValue(name)
		k.Close()
		if err == nil {
			return uint32(v), true
		}
	}
	return 0, false
}

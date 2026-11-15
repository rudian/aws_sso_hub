//go:build !windows && !(darwin && cgo)

package watch

type nativeBackend struct{}

func newNativeBackend() backend { return nativeBackend{} }

func (nativeBackend) resolve(root string) (string, error) { return "", ErrUnsupported }
func (nativeBackend) start(string, string, emitFunc) (func(), error) {
	return nil, ErrUnsupported
}

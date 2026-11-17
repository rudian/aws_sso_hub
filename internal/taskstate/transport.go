package taskstate

import (
	"errors"
	"io"
	"time"
)

// ErrInUse is returned by Listen when a live app already serves the endpoint.
var ErrInUse = errors.New("taskstate: endpoint in use")

// listener is the platform half of the socket: Accept returns a connection that has already passed the
// same-user check.
type listener interface {
	Accept() (io.ReadCloser, error)
	Close() error
}

const (
	dialTimeout  = 300 * time.Millisecond
	readTimeout  = 2 * time.Second
	closeTimeout = 2 * time.Second
)

// Send writes one report to the app and returns at once. It never waits for the app to act: a missing app is
// an error the caller ignores.
func Send(endpoint string, r Report) error {
	b, err := marshalReport(r)
	if err != nil {
		return err
	}
	return dialSend(endpoint, b)
}

//go:build windows

package watch

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const winMask = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_ATTRIBUTES | windows.FILE_NOTIFY_CHANGE_SIZE |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE | windows.FILE_NOTIFY_CHANGE_CREATION

// winReadRetries bounds how often a failed re-issue of the read is retried
// (100 ms, 200 ms, 400 ms ...) before the watch is reported as stopped.
const winReadRetries = 5

// winQuitKey is the completion key that ends the dispatcher goroutine.
const winQuitKey = 0

var errWinClosed = errors.New("watch: backend is closed")

// nativeBackend watches folders with ReadDirectoryChangesW. All directory
// handles of one Watcher are bound to one I/O completion port that one
// goroutine drains, so N watched folders cost N handles and one OS thread, not
// N threads (a blocked, pinned thread per folder costs several kernel handles
// each on Windows 10 and later). Reads on a handle bound to a port are not
// cancelled when the thread that issued them exits, so no goroutine is pinned.
//
// The port and its goroutine are created by the first start and released by
// close, which waits until every watch has been stopped.
type nativeBackend struct {
	mu      sync.Mutex
	port    windows.Handle
	watches map[uintptr]*winWatch
	nextKey uintptr
	closed  bool
	active  sync.WaitGroup
}

func newNativeBackend() backend {
	return &nativeBackend{watches: map[uintptr]*winWatch{}, nextKey: winQuitKey}
}

func (*nativeBackend) resolve(root string) (string, error) { return root, nil }

// winWatch is the memory the kernel writes into while a read is pending. It is
// allocated on the heap, once per watch, and stays in nativeBackend.watches
// until the read has completed or been cancelled. Goroutine stacks can move
// while the kernel still holds pointers into them; the heap never moves. Do
// not turn these fields into locals.
type winWatch struct {
	ov  windows.Overlapped // first field, zeroed before every read
	buf [16 * 1024]uint32  // 64 KiB, DWORD aligned

	b    *nativeBackend
	key  uintptr
	h    windows.Handle
	root string
	emit emitFunc

	mu      sync.Mutex // guards the fields below; emit is called with it held
	reading bool       // a read is pending in the kernel
	stopped bool       // stop was requested: never issue or emit again
	aborted chan struct{}
	quit    chan struct{} // closed by stop to end a retry wait early
}

// port returns the completion port, creating it and its goroutine on first use.
// Caller holds b.mu.
func (b *nativeBackend) portLocked() (windows.Handle, error) {
	if b.port != 0 {
		return b.port, nil
	}
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return 0, err
	}
	b.port = port
	go b.dispatch(port)
	return port, nil
}

// start opens one directory handle for the whole tree (bWatchSubtree), binds it
// to the port and issues the first read before returning, so a file system that
// refuses change notifications fails Subscribe instead of failing silently
// later. stop cancels the read, waits for its completion and closes the handle,
// so the folder can be renamed afterwards.
func (b *nativeBackend) start(root, _ string, emit emitFunc) (func(), error) {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		windows.CloseHandle(h)
		return nil, errWinClosed
	}
	port, err := b.portLocked()
	if err != nil {
		b.mu.Unlock()
		windows.CloseHandle(h)
		return nil, err
	}
	b.nextKey++
	ww := &winWatch{b: b, key: b.nextKey, h: h, root: root, emit: emit,
		aborted: make(chan struct{}), quit: make(chan struct{})}
	if _, err := windows.CreateIoCompletionPort(h, port, ww.key, 0); err != nil {
		b.mu.Unlock()
		windows.CloseHandle(h)
		return nil, err
	}
	b.watches[ww.key] = ww
	b.active.Add(1)
	b.mu.Unlock()

	ww.mu.Lock()
	err = ww.issueLocked()
	ww.mu.Unlock()
	if err != nil {
		ww.release()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(ww.stop) }, nil
}

// issueLocked starts one asynchronous read into ww. Caller holds ww.mu.
func (ww *winWatch) issueLocked() error {
	if ww.stopped {
		return errWinClosed
	}
	ww.ov = windows.Overlapped{}
	err := windows.ReadDirectoryChanges(ww.h, (*byte)(unsafe.Pointer(&ww.buf[0])),
		uint32(len(ww.buf)*4), true, winMask, nil, &ww.ov, 0)
	if err == nil {
		ww.reading = true
	}
	return err
}

// stop ends the watch. When a read is pending it is cancelled and its
// completion (ERROR_OPERATION_ABORTED, or the data that raced with the cancel)
// is awaited, because the kernel owns the buffer until then. After stop returns
// no emit call starts and the handle is closed.
func (ww *winWatch) stop() {
	ww.mu.Lock()
	ww.stopped = true
	wasReading := ww.reading
	close(ww.quit)
	ww.mu.Unlock()
	if wasReading {
		windows.CancelIoEx(ww.h, &ww.ov) // ERROR_NOT_FOUND: it completed already; its completion is queued
		<-ww.aborted
	}
	ww.release()
}

// release closes the handle and forgets the watch.
func (ww *winWatch) release() {
	windows.CloseHandle(ww.h)
	ww.b.mu.Lock()
	delete(ww.b.watches, ww.key)
	ww.b.mu.Unlock()
	ww.b.active.Done()
}

// close ends the dispatcher once every watch has been stopped, and refuses new
// watches.
func (b *nativeBackend) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.active.Wait()
	b.mu.Lock()
	port := b.port
	b.port = 0 // dispatch closes the handle after the quit packet; never post twice
	b.mu.Unlock()
	if port != 0 {
		windows.PostQueuedCompletionStatus(port, 0, winQuitKey, nil)
	}
}

// dispatch is the one goroutine that drains the port.
func (b *nativeBackend) dispatch(port windows.Handle) {
	defer windows.CloseHandle(port)
	for {
		var n uint32
		var key uintptr
		var ov *windows.Overlapped
		err := windows.GetQueuedCompletionStatus(port, &n, &key, &ov, windows.INFINITE)
		if ov == nil {
			if err != nil || key == winQuitKey {
				return // the port failed, or close posted the quit packet
			}
			continue
		}
		b.mu.Lock()
		ww := b.watches[key]
		b.mu.Unlock()
		if ww != nil {
			ww.complete(n, err)
		}
	}
}

// complete handles one finished read: it emits the notifications and issues the
// next read. err is the I/O status (ERROR_NOTIFY_ENUM_DIR, ERROR_OPERATION_ABORTED ...).
func (ww *winWatch) complete(n uint32, ioErr error) {
	ww.mu.Lock()
	defer ww.mu.Unlock()
	ww.reading = false
	if ww.stopped {
		close(ww.aborted)
		return
	}
	switch {
	case ioErr != nil:
		// ERROR_NOTIFY_ENUM_DIR or a transient failure: changes may have been
		// lost. Report it and read again.
		ww.emit(nil, true, false, false)
	case n == 0:
		ww.emit(nil, true, false, false) // buffer overflowed: everything may have changed
	default:
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(&ww.buf[0])), len(ww.buf)*4)
		var paths []string
		for off := uint32(0); off < n; {
			fni := (*windows.FileNotifyInformation)(unsafe.Pointer(&bytes[off]))
			name := utf16.Decode(unsafe.Slice(&fni.FileName, fni.FileNameLength/2))
			paths = append(paths, filepath.Join(ww.root, string(name)))
			if fni.NextEntryOffset == 0 {
				break
			}
			off += fni.NextEntryOffset
		}
		ww.emit(paths, false, false, false)
	}
	if ww.issueLocked() == nil {
		return
	}
	// The read could not be re-issued. Retry in the background so the one
	// dispatcher goroutine never sleeps.
	go ww.retry()
}

// retry re-issues the read with a bounded backoff while the root still exists.
// It ends the watch (rootGone and dead, or dead) when that fails for good. No
// read is pending while it runs.
func (ww *winWatch) retry() {
	delay := 100 * time.Millisecond
	for attempt := 0; ; attempt++ {
		if _, err := os.Stat(ww.root); err != nil {
			ww.finish(true)
			return
		}
		if attempt >= winReadRetries {
			ww.finish(false)
			return
		}
		t := time.NewTimer(delay)
		select {
		case <-ww.quit:
			t.Stop()
			return
		case <-t.C:
		}
		delay *= 2
		ww.mu.Lock()
		if ww.stopped {
			ww.mu.Unlock()
			return
		}
		err := ww.issueLocked()
		if err == nil {
			// Changes made while no read was pending are lost: tell the
			// subscribers to rescan.
			ww.emit(nil, true, false, false)
		}
		ww.mu.Unlock()
		if err == nil {
			return
		}
	}
}

// finish reports that the watch is over. dead is always set, so the Watcher
// ends the record as Stopped (or Removed when the path is gone) and a later
// Subscribe can start it again.
func (ww *winWatch) finish(rootGone bool) {
	ww.mu.Lock()
	defer ww.mu.Unlock()
	if ww.stopped {
		return
	}
	ww.emit(nil, true, rootGone, true)
}

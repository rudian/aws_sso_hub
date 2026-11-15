//go:build darwin && cgo

package watch

/*
#cgo LDFLAGS: -framework CoreServices -framework CoreFoundation
#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <stdlib.h>
#include <fcntl.h>
#include <limits.h>
#include <unistd.h>

extern void goFSEventsCallback(uintptr_t h, size_t n, char **paths, FSEventStreamEventFlags *flags);

static void fsCallback(ConstFSEventStreamRef s, void *info, size_t n, void *paths,
                       const FSEventStreamEventFlags flags[], const FSEventStreamEventId ids[]) {
	goFSEventsCallback((uintptr_t)info, n, (char **)paths, (FSEventStreamEventFlags *)flags);
}

typedef struct {
	FSEventStreamRef stream;
	dispatch_queue_t queue;
} fsHandle;

static fsHandle *fsStart(const char *path, uintptr_t h, double latency) {
	CFStringRef p = CFStringCreateWithCString(NULL, path, kCFStringEncodingUTF8);
	if (!p) return NULL;
	CFArrayRef paths = CFArrayCreate(NULL, (const void **)&p, 1, &kCFTypeArrayCallBacks);
	CFRelease(p);
	FSEventStreamContext ctx = {0, (void *)h, NULL, NULL, NULL};
	FSEventStreamRef s = FSEventStreamCreate(NULL, fsCallback, &ctx, paths,
		kFSEventStreamEventIdSinceNow, latency,
		kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagNoDefer);
	CFRelease(paths);
	if (!s) return NULL;
	fsHandle *fh = (fsHandle *)malloc(sizeof(fsHandle));
	fh->stream = s;
	fh->queue = dispatch_queue_create("codepass.watch", DISPATCH_QUEUE_SERIAL);
	FSEventStreamSetDispatchQueue(s, fh->queue);
	if (!FSEventStreamStart(s)) {
		FSEventStreamInvalidate(s);
		FSEventStreamRelease(s);
		dispatch_release(fh->queue);
		free(fh);
		return NULL;
	}
	return fh;
}

static void fsStop(fsHandle *fh) {
	FSEventStreamStop(fh->stream);
	FSEventStreamInvalidate(fh->stream);
	FSEventStreamRelease(fh->stream);
	// Wait for a callback that is already running on the queue.
	dispatch_sync(fh->queue, ^{});
	dispatch_release(fh->queue);
	free(fh);
}

// fsRealPath returns the path of the open descriptor as the file system spells it.
static int fsRealPath(int fd, char *buf) { return fcntl(fd, F_GETPATH, buf); }

static int fsOverflow(FSEventStreamEventFlags f) {
	return (f & (kFSEventStreamEventFlagMustScanSubDirs | kFSEventStreamEventFlagUserDropped |
	             kFSEventStreamEventFlagKernelDropped | kFSEventStreamEventFlagEventIdsWrapped)) != 0;
}
static int fsRoot(FSEventStreamEventFlags f) { return (f & kFSEventStreamEventFlagRootChanged) != 0; }
static char *fsPath(char **paths, size_t i) { return paths[i]; }
static FSEventStreamEventFlags fsFlag(FSEventStreamEventFlags *f, size_t i) { return f[i]; }
*/
import "C"

import (
	"errors"
	"os"
	"path/filepath"
	"runtime/cgo"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

type nativeBackend struct{}

type fsStream struct {
	mu       sync.Mutex
	emit     emitFunc
	root     string // as the caller spelled it
	realRoot string // as FSEvents reports it
	stopped  bool
}

func newNativeBackend() backend { return nativeBackend{} }

// liveStreams counts FSEvents streams started and not yet stopped (tests read it).
var liveStreams atomic.Int32

// resolve returns the folder's path spelled as the disk spells it (letter case,
// Unicode form), which is how FSEvents reports paths. It falls back to
// EvalSymlinks when the descriptor path is not available.
func (nativeBackend) resolve(root string) (string, error) {
	d, err := os.Open(root)
	if err != nil {
		return "", err
	}
	defer d.Close()
	buf := (*C.char)(C.malloc(C.PATH_MAX + 1))
	defer C.free(unsafe.Pointer(buf))
	if C.fsRealPath(C.int(d.Fd()), buf) == 0 {
		if p := C.GoString(buf); p != "" {
			return p, nil
		}
	}
	return filepath.EvalSymlinks(root)
}

func (nativeBackend) start(root, real string, emit emitFunc) (func(), error) {
	s := &fsStream{emit: emit, root: root, realRoot: real}
	h := cgo.NewHandle(s)
	cpath := C.CString(real)
	defer C.free(unsafe.Pointer(cpath))
	fh := C.fsStart(cpath, C.uintptr_t(h), 0.05)
	if fh == nil {
		h.Delete()
		return nil, errors.New("watch: FSEventStreamStart failed for " + root)
	}
	liveStreams.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.stopped = true
			s.mu.Unlock()
			C.fsStop(fh) // returns after the last callback finished
			liveStreams.Add(-1)
			h.Delete()
		})
	}, nil
}

//export goFSEventsCallback
func goFSEventsCallback(h C.uintptr_t, n C.size_t, paths **C.char, flags *C.FSEventStreamEventFlags) {
	s, ok := cgo.Handle(h).Value().(*fsStream)
	if !ok {
		return
	}
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return
	}
	out := make([]string, 0, int(n))
	overflow, rootGone := false, false
	for i := C.size_t(0); i < n; i++ {
		f := C.fsFlag(flags, i)
		if C.fsOverflow(f) != 0 {
			overflow = true
		}
		if C.fsRoot(f) != 0 {
			rootGone = true
		}
		p := C.GoString(C.fsPath(paths, i))
		if p == s.realRoot {
			p = s.root
		} else if strings.HasPrefix(p, s.realRoot+"/") {
			p = s.root + p[len(s.realRoot):]
		}
		out = append(out, p)
	}
	s.emit(out, overflow, rootGone, false)
}

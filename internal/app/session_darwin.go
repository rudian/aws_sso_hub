//go:build darwin

package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// codepassQuitIsSessionEnd reports whether the Apple Event being handled is a quit sent for a logout, restart
// or shutdown (its keyAEQuitReason 'why?' is one of those). Call it only on the main thread, from inside
// applicationShouldTerminate, where the current event is the quit event. A plain Cmd+Q has no such event.
static volatile int codepassPoweringOff = 0;

// codepassWatchPowerOff records NSWorkspaceWillPowerOffNotification (logout, restart and shutdown): a second
// signal in case the Apple Event carries no readable reason.
static void codepassWatchPowerOff(void) {
	[[[NSWorkspace sharedWorkspace] notificationCenter]
		addObserverForName:NSWorkspaceWillPowerOffNotification object:nil queue:nil
		usingBlock:^(NSNotification *n) { codepassPoweringOff = 1; }];
}

static int codepassQuitIsSessionEnd(void) {
	if (codepassPoweringOff) return 1;
	NSAppleEventDescriptor *ev = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
	if (ev == nil) return 0;
	// loginwindow may send the reason as an attribute or as a parameter: read both.
	NSAppleEventDescriptor *why = [ev attributeDescriptorForKeyword:'why?'];
	if (why == nil) why = [ev paramDescriptorForKeyword:'why?'];
	if (why == nil) return 0;
	switch ([why enumCodeValue]) {
	case 'logo': case 'rlgo': // log out, really log out
	case 'rest': case 'rrst': // restart, restart dialog
	case 'shut': case 'rsdn': // shut down, shut down dialog
		return 1;
	}
	return 0;
}
*/
import "C"

func init() { C.codepassWatchPowerOff() }

func sessionEnding() bool { return C.codepassQuitIsSessionEnd() != 0 }

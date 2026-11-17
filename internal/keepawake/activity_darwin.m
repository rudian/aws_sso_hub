//go:build darwin && cgo

#import <Foundation/Foundation.h>
#include <stdint.h>

// The activities begun here, kept alive until ended. Handles are 1, 2, 3, ... (0 = failure).
static NSMutableDictionary<NSNumber *, id<NSObject>> *activities;
static uint64_t nextHandle = 1;

uint64_t keepawake_begin(const char *reason) {
	@autoreleasepool {
		@synchronized([NSProcessInfo class]) {
			if (activities == nil) activities = [NSMutableDictionary new];
			// NSActivityUserInitiatedAllowingIdleSystemSleep: no App Nap (a hidden window is not throttled), but
			// idle sleep stays allowed: it is the keep-awake setting that decides about sleep, and caffeinate -i
			// (-d for the screen) holds that. The activity is wanted whenever a task counts, setting on or off.
			id<NSObject> a = [[NSProcessInfo processInfo]
				beginActivityWithOptions:NSActivityUserInitiatedAllowingIdleSystemSleep
				reason:[NSString stringWithUTF8String:reason]];
			if (a == nil) return 0;
			uint64_t h = nextHandle++;
			activities[@(h)] = a;
			return h;
		}
	}
}

void keepawake_end(uint64_t handle) {
	@autoreleasepool {
		@synchronized([NSProcessInfo class]) {
			id<NSObject> a = activities[@(handle)];
			if (a == nil) return;
			[[NSProcessInfo processInfo] endActivity:a];
			[activities removeObjectForKey:@(handle)];
		}
	}
}

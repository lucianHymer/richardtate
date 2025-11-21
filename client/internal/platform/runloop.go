package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Run the Cocoa event loop on the main thread
// This is required for Carbon Events (hotkeys) to be delivered
static void runCocoaEventLoop() {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp run];
    }
}
*/
import "C"

// RunEventLoop runs the Cocoa event loop (blocks forever)
// This is required for Carbon Events hotkeys to work
func RunEventLoop() {
	C.runCocoaEventLoop()
}

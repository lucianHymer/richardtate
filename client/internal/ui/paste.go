package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices

#include <ApplicationServices/ApplicationServices.h>

static void simulatePaste() {
    // Create key down event for Cmd+V
    CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 9, true);  // 9 = 'v' key code
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);

    // Create key up event
    CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, 9, false);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    // Post events to the system
    CGEventPost(kCGHIDEventTap, keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);

    // Release the events
    CFRelease(keyDown);
    CFRelease(keyUp);
}
*/
import "C"

import (
	"golang.design/x/clipboard"
)

// InitClipboard initializes the clipboard (call once at startup)
func InitClipboard() error {
	return clipboard.Init()
}

// PasteText copies text to clipboard and simulates Cmd+V
func PasteText(text string) {
	// Write to clipboard
	clipboard.Write(clipboard.FmtText, []byte(text))

	// Small delay to ensure clipboard is ready would go here if needed
	// but typically not required

	// Simulate Cmd+V
	C.simulatePaste()
}

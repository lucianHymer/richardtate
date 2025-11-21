package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices

#include <ApplicationServices/ApplicationServices.h>
#include <stdbool.h>

static bool checkAccessibilityPermissions() {
    return AXIsProcessTrusted();
}

static void promptForAccessibilityPermissions() {
    // Create options dict with prompt option
    const void* keys[] = { kAXTrustedCheckOptionPrompt };
    const void* values[] = { kCFBooleanTrue };

    CFDictionaryRef options = CFDictionaryCreate(
        kCFAllocatorDefault,
        keys,
        values,
        1,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks
    );

    AXIsProcessTrustedWithOptions(options);
    CFRelease(options);
}
*/
import "C"

import (
	"fmt"
	"time"
)

// EnsureAccessibilityPermissions checks if accessibility permissions are granted
func EnsureAccessibilityPermissions() bool {
	return bool(C.checkAccessibilityPermissions())
}

// WaitForAccessibilityPermissions waits for the user to grant permissions
func WaitForAccessibilityPermissions(onWait func()) {
	// Prompt once
	C.promptForAccessibilityPermissions()

	// Callback to inform user
	if onWait != nil {
		onWait()
	}

	// Poll until granted (check every second)
	for {
		time.Sleep(1 * time.Second)
		if EnsureAccessibilityPermissions() {
			return
		}
	}
}

// PromptForAccessibilityPermissions opens System Preferences to grant permissions
func PromptForAccessibilityPermissions() {
	C.promptForAccessibilityPermissions()
}

// MustHaveAccessibilityPermissions checks and exits if permissions not granted
func MustHaveAccessibilityPermissions() {
	if !EnsureAccessibilityPermissions() {
		fmt.Println("⚠️  Accessibility permissions required for hotkeys and text pasting")
		fmt.Println("Opening System Preferences...")
		WaitForAccessibilityPermissions(func() {
			fmt.Println("Please grant accessibility permissions in System Preferences")
			fmt.Println("(Privacy & Security → Accessibility → richardtate-client)")
		})
		fmt.Println("✅ Accessibility permissions granted!")
	}
}

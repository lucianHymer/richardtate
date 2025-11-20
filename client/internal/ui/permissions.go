package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit

#include <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>

// Check if we have accessibility permissions
static int checkAccessibilityPermissions() {
    return AXIsProcessTrusted();
}

// Prompt user for accessibility permissions
// Opens System Preferences to the right pane
static void promptForAccessibility() {
    NSDictionary *options = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt: @YES};
    AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
}

// Open System Preferences directly to Accessibility pane
static void openAccessibilityPreferences() {
    NSString *urlString = @"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility";
    NSURL *url = [NSURL URLWithString:urlString];
    [[NSWorkspace sharedWorkspace] openURL:url];
}
*/
import "C"

import (
	"time"
)

// HasAccessibilityPermissions checks if the app has accessibility permissions
func HasAccessibilityPermissions() bool {
	return C.checkAccessibilityPermissions() != 0
}

// RequestAccessibilityPermissions prompts the user for accessibility permissions
// This shows the system dialog and opens System Preferences
func RequestAccessibilityPermissions() {
	C.promptForAccessibility()
}

// OpenAccessibilityPreferences opens System Preferences to the Accessibility pane
func OpenAccessibilityPreferences() {
	C.openAccessibilityPreferences()
}

// EnsureAccessibilityPermissions checks permissions and prompts if needed
// Returns true if permissions are granted, false if user needs to grant them
func EnsureAccessibilityPermissions() bool {
	if HasAccessibilityPermissions() {
		return true
	}

	// Prompt user - this shows system dialog
	RequestAccessibilityPermissions()

	// Give user time to respond (the dialog is async)
	// We'll check again after a short delay
	time.Sleep(500 * time.Millisecond)

	return HasAccessibilityPermissions()
}

// WaitForAccessibilityPermissions blocks until permissions are granted
// Shows a message and polls periodically
func WaitForAccessibilityPermissions(onWaiting func()) {
	if HasAccessibilityPermissions() {
		return
	}

	// Show initial prompt
	RequestAccessibilityPermissions()

	// Call the waiting callback if provided
	if onWaiting != nil {
		onWaiting()
	}

	// Poll until permissions are granted
	for !HasAccessibilityPermissions() {
		time.Sleep(1 * time.Second)
	}
}

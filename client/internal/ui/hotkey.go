package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Carbon -framework Cocoa

#include <Carbon/Carbon.h>

// Forward declaration for Go callback
extern void goHotkeyCallback(int id);

// Carbon Event handler for hotkeys
static OSStatus hotkeyHandler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
    EventHotKeyID hotKeyID;
    GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof(hotKeyID), NULL, &hotKeyID);
    goHotkeyCallback(hotKeyID.id);
    return noErr;
}

// Register a global hotkey with Carbon Event Manager
static void registerHotkey(int keyCode, int modifiers, int id) {
    EventHotKeyID hotKeyID = {'htky', id};
    EventHotKeyRef hotKeyRef;
    RegisterEventHotKey(keyCode, modifiers, hotKeyID, GetApplicationEventTarget(), 0, &hotKeyRef);
}

// Install the event handler for hotkey events
static void installHotkeyHandler() {
    EventTypeSpec eventType = {kEventClassKeyboard, kEventHotKeyPressed};
    InstallApplicationEventHandler(&hotkeyHandler, 1, &eventType, NULL, NULL);
}
*/
import "C"

import (
	"sync"

	"github.com/progrium/darwinkit/dispatch"
)

var (
	toggleCallback      func()
	calibrationCallback func()
	hotkeyMu            sync.Mutex
)

//export goHotkeyCallback
func goHotkeyCallback(id C.int) {
	hotkeyMu.Lock()
	var cb func()
	switch id {
	case 1:
		cb = toggleCallback
	case 2:
		cb = calibrationCallback
	}
	hotkeyMu.Unlock()

	if cb != nil {
		// CRITICAL: Dispatch to main thread for UI operations
		dispatch.MainQueue().DispatchAsync(cb)
	}
}

// RegisterHotkeys registers both global hotkeys
// Ctrl+N (toggle recording) and Ctrl+Alt+C (calibration)
func RegisterHotkeys(onToggle, onCalibration func()) {
	hotkeyMu.Lock()
	toggleCallback = onToggle
	calibrationCallback = onCalibration
	hotkeyMu.Unlock()

	// Install the event handler first
	C.installHotkeyHandler()

	// Key codes: https://gist.github.com/eegrok/949034
	// 'n' = 45, 'c' = 8

	// Modifiers (Carbon):
	// control = 0x1000 (controlKey)
	// option  = 0x0800 (optionKey)
	// command = 0x0100 (cmdKey)
	// shift   = 0x0200 (shiftKey)

	// Hotkey 1: Ctrl+N (toggle recording)
	C.registerHotkey(45, 0x1000, 1)

	// Hotkey 2: Ctrl+Alt+C (calibration)
	// Control + Option = 0x1000 | 0x0800 = 0x1800
	C.registerHotkey(8, 0x1800, 2)
}

// Common key codes for reference:
// 'n' = 45, 'c' = 8, 'v' = 9, 'space' = 49
// Modifiers: control = 0x1000, option = 0x0800, command = 0x0100, shift = 0x0200

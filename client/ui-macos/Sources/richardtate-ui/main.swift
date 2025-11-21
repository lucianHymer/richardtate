import Cocoa
import Foundation

class AppDelegate: NSObject, NSApplicationDelegate {
    var previewWindow: PreviewWindow!
    var calibrationWindow: CalibrationWindow!

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Create windows (initially hidden)
        previewWindow = PreviewWindow()
        calibrationWindow = CalibrationWindow()

        // Print ready signal
        print("READY", to: &standardError)
        fflush(stderr)

        // Start reading commands from stdin
        startCommandLoop()
    }

    func startCommandLoop() {
        DispatchQueue.global(qos: .userInitiated).async {
            while let line = readLine() {
                guard let data = line.data(using: .utf8),
                      let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let command = json["command"] as? String else {
                    continue
                }

                // Execute on main thread (AppKit requirement)
                DispatchQueue.main.async {
                    self.handleCommand(command, json: json)
                }
            }

            // stdin closed, exit
            NSApplication.shared.terminate(nil)
        }
    }

    func handleCommand(_ command: String, json: [String: Any]) {
        switch command {
        // Preview window commands
        case "show":
            previewWindow.makeKeyAndOrderFront(nil)
        case "hide":
            previewWindow.orderOut(nil)
        case "setText":
            if let text = json["text"] as? String {
                previewWindow.setText(text)
            }
        case "setProcessing":
            if let processing = json["processing"] as? Bool {
                previewWindow.setProcessing(processing)
            }
        case "clearText":
            previewWindow.clearText()

        // Calibration window commands
        case "showCalibration":
            calibrationWindow.makeKeyAndOrderFront(nil)
        case "hideCalibration":
            calibrationWindow.orderOut(nil)
        case "setCalibrationStep":
            if let step = json["step"] as? Int {
                calibrationWindow.setStep(step)
            }
        case "setCalibrationMessage":
            if let message = json["message"] as? String {
                calibrationWindow.setMessage(message)
            }
        case "setCalibrationProgress":
            if let value = json["value"] as? Double {
                calibrationWindow.setProgress(value)
            }
        case "setCalibrationStats":
            if let bg = json["backgroundP95"] as? Double,
               let sp = json["speechP5"] as? Double,
               let rec = json["recommended"] as? Double {
                calibrationWindow.setStats(
                    backgroundP95: bg,
                    speechP5: sp,
                    recommended: rec
                )
            }

        // Lifecycle
        case "quit":
            NSApplication.shared.terminate(nil)

        default:
            print("Unknown command: \(command)", to: &standardError)
        }
    }
}

// Helper for stderr output
var standardError = FileHandle.standardError

extension FileHandle: @retroactive TextOutputStream {
    public func write(_ string: String) {
        guard let data = string.data(using: .utf8) else { return }
        self.write(data)
    }
}

// Main entry point
let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()

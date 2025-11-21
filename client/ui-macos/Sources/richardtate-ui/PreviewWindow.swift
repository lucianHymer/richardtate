import Cocoa

class PreviewWindow: NSWindow {
    private let textField: NSTextField
    private var displayText: String = ""
    private var isProcessing: Bool = false

    init() {
        // Window configuration
        let frame = NSRect(x: 0, y: 0, width: 400, height: 200)
        super.init(
            contentRect: frame,
            styleMask: [.titled, .resizable],  // No close button
            backing: .buffered,
            defer: false
        )

        // Window properties
        self.title = "Dictation Preview"
        self.level = .floating
        self.backgroundColor = NSColor(red: 0.1, green: 0.1, blue: 0.1, alpha: 0.95)
        self.isMovableByWindowBackground = true
        self.ignoresMouseEvents = true  // Click-through
        self.center()

        // Create text field (wrapping label)
        textField = NSTextField(wrappingLabelWithString: "")
        textField.frame = NSRect(x: 10, y: 10, width: 380, height: 180)
        textField.textColor = NSColor(red: 0.4, green: 0.95, blue: 0.7, alpha: 1.0)
        textField.font = NSFont.systemFont(ofSize: 14)
        textField.alignment = .left

        contentView?.addSubview(textField)
    }

    func setText(_ text: String) {
        displayText = text
        updateDisplay()
    }

    func setProcessing(_ processing: Bool) {
        isProcessing = processing
        updateDisplay()
    }

    func clearText() {
        displayText = ""
        isProcessing = false
        updateDisplay()
    }

    private func updateDisplay() {
        var display = displayText

        // Append "..." if processing
        if isProcessing {
            display += "..."
        }

        // Limit to last 500 characters
        if display.count > 500 {
            display = "..." + String(display.suffix(497))
        }

        textField.stringValue = display
    }
}

import Cocoa

class CalibrationWindow: NSWindow {
    private let messageLabel: NSTextField
    private let progressIndicator: NSProgressIndicator
    private let statsLabel: NSTextField
    private let backgroundBar: NSView
    private let speechBar: NSView
    private var currentStep: Int = 0

    init() {
        // Window configuration
        let frame = NSRect(x: 0, y: 0, width: 500, height: 400)
        super.init(
            contentRect: frame,
            styleMask: [.titled],  // No close button
            backing: .buffered,
            defer: false
        )

        self.title = "VAD Calibration"
        self.center()

        // Create UI elements
        messageLabel = NSTextField(labelWithString: "")
        messageLabel.frame = NSRect(x: 20, y: 320, width: 460, height: 60)
        messageLabel.font = NSFont.systemFont(ofSize: 16)
        messageLabel.alignment = .center

        progressIndicator = NSProgressIndicator()
        progressIndicator.frame = NSRect(x: 100, y: 280, width: 300, height: 20)
        progressIndicator.style = .bar
        progressIndicator.minValue = 0
        progressIndicator.maxValue = 1
        progressIndicator.isIndeterminate = false

        statsLabel = NSTextField(wrappingLabelWithString: "")
        statsLabel.frame = NSRect(x: 20, y: 100, width: 460, height: 160)
        statsLabel.font = NSFont.monospacedSystemFont(ofSize: 12, weight: .regular)

        backgroundBar = NSView(frame: NSRect(x: 100, y: 60, width: 0, height: 20))
        backgroundBar.wantsLayer = true
        backgroundBar.layer?.backgroundColor = NSColor.systemBlue.cgColor

        speechBar = NSView(frame: NSRect(x: 100, y: 30, width: 0, height: 20))
        speechBar.wantsLayer = true
        speechBar.layer?.backgroundColor = NSColor.systemGreen.cgColor

        contentView?.addSubview(messageLabel)
        contentView?.addSubview(progressIndicator)
        contentView?.addSubview(statsLabel)
        contentView?.addSubview(backgroundBar)
        contentView?.addSubview(speechBar)
    }

    func setStep(_ step: Int) {
        currentStep = step

        // Reset visibility for step transitions
        progressIndicator.isHidden = false
        statsLabel.isHidden = true
        backgroundBar.isHidden = true
        speechBar.isHidden = true

        switch step {
        case 1:
            self.backgroundColor = NSColor(red: 0.2, green: 0.4, blue: 0.8, alpha: 1.0)
        case 2:
            self.backgroundColor = NSColor(red: 0.8, green: 0.5, blue: 0.2, alpha: 1.0)
        case 3:
            self.backgroundColor = NSColor(red: 0.2, green: 0.7, blue: 0.3, alpha: 1.0)
            progressIndicator.isHidden = true
            statsLabel.isHidden = false
            backgroundBar.isHidden = false
            speechBar.isHidden = false
        default:
            break
        }
    }

    func setMessage(_ message: String) {
        messageLabel.stringValue = message
    }

    func setProgress(_ value: Double) {
        progressIndicator.doubleValue = value
    }

    func setStats(backgroundP95: Double, speechP5: Double, recommended: Double) {
        let stats = """
        Analysis Results:

        Background Noise P95: \(String(format: "%.1f", backgroundP95))
        Speech P5: \(String(format: "%.1f", speechP5))

        Recommended Threshold: \(String(format: "%.1f", recommended))
        """
        statsLabel.stringValue = stats

        // Update bar widths (scale to 300px max)
        let maxWidth: CGFloat = 300
        let scale = maxWidth / max(backgroundP95, speechP5)
        backgroundBar.frame.size.width = CGFloat(backgroundP95) * scale
        speechBar.frame.size.width = CGFloat(speechP5) * scale
    }
}

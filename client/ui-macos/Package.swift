// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "richardtate-ui",
    platforms: [.macOS(.v11)],
    products: [
        .executable(name: "richardtate-ui", targets: ["richardtate-ui"])
    ],
    targets: [
        .executableTarget(name: "richardtate-ui")
    ]
)

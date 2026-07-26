// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "ChatDomain",
    platforms: [
        .iOS(.v17),
        .macOS(.v14),
    ],
    products: [
        .library(name: "ChatDomain", targets: ["ChatDomain"]),
    ],
    targets: [
        .target(name: "ChatDomain"),
        .testTarget(
            name: "ChatDomainTests",
            dependencies: ["ChatDomain"]
        ),
    ]
)

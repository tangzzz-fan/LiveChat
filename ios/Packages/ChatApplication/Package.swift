// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "ChatApplication",
    platforms: [
        .iOS(.v17),
        .macOS(.v14),
    ],
    products: [
        .library(name: "ChatApplication", targets: ["ChatApplication"]),
    ],
    dependencies: [
        .package(path: "../ChatDomain"),
        .package(path: "../ChatInfrastructure"),
    ],
    targets: [
        .target(
            name: "ChatApplication",
            dependencies: ["ChatDomain", "ChatInfrastructure"]
        ),
        .testTarget(
            name: "ChatApplicationTests",
            dependencies: ["ChatApplication"]
        ),
    ]
)

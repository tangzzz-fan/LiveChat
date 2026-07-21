// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "ChatDomain",
    platforms: [.iOS(.v16)],
    products: [
        .library(name: "ChatDomain", targets: ["ChatDomain"]),
    ],
    targets: [
        .target(name: "ChatDomain"),
    ]
)

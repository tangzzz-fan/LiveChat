// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "ChatInfrastructure",
    platforms: [.iOS(.v16)],
    products: [
        .library(name: "ChatInfrastructure", targets: ["ChatInfrastructure"]),
    ],
    dependencies: [
        .package(path: "../ChatDomain"),
        .package(url: "https://github.com/groue/GRDB.swift.git", from: "6.29.0"),
    ],
    targets: [
        .target(
            name: "ChatInfrastructure",
            dependencies: ["ChatDomain", .product(name: "GRDB", package: "GRDB.swift")]
        ),
    ]
)

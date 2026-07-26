// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "ChatInfrastructure",
    platforms: [
        .iOS(.v17),
        .macOS(.v14),
    ],
    products: [
        .library(name: "ChatInfrastructure", targets: ["ChatInfrastructure"]),
    ],
    dependencies: [
        .package(path: "../ChatDomain"),
        .package(path: "../../../../TG Libraries/GRDB.swift"),
        .package(path: "../../../../TG Libraries/swift-protobuf"),
    ],
    targets: [
        .target(
            name: "ChatInfrastructure",
            dependencies: [
                "ChatDomain",
                .product(name: "GRDB", package: "GRDB.swift"),
                .product(name: "SwiftProtobuf", package: "swift-protobuf"),
            ]
        ),
        .testTarget(
            name: "ChatInfrastructureTests",
            dependencies: ["ChatInfrastructure"]
        ),
    ]
)

// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "ChatPresentation",
    platforms: [
        .iOS(.v17),
        .macOS(.v14),
    ],
    products: [
        .library(name: "ChatPresentation", targets: ["ChatPresentation"]),
    ],
    dependencies: [
        .package(path: "../ChatApplication"),
        .package(path: "../ChatDomain"),
        .package(path: "../../../../TG Libraries/TGReduxKit"),
    ],
    targets: [
        .target(
            name: "ChatPresentation",
            dependencies: [
                "ChatApplication",
                "ChatDomain",
                .product(name: "TGReduxKit", package: "TGReduxKit"),
            ]
        ),
        .testTarget(
            name: "ChatPresentationTests",
            dependencies: ["ChatPresentation", "ChatApplication", "ChatDomain"]
        ),
    ]
)

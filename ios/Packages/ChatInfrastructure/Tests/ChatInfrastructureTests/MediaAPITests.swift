import Testing
import Foundation
import ChatDomain
@testable import ChatInfrastructure

@Test
func imageMessageContentEncodesAttachmentShape() throws {
    let attachment = Attachment(
        objectKey: "media/u_1/img.jpg",
        mimeType: "image/jpeg",
        sizeBytes: 12_345,
        width: 100,
        height: 80
    )
    let json = try ImageMessageContent.encodeAttachment(attachment)
    #expect(ImageMessageContent.parseObjectKey(from: json) == "media/u_1/img.jpg")
    #expect(json.contains("image/jpeg") || json.contains("image\\/jpeg"))
    #expect(json.contains("12345"))
}

@Test
func imageMediaCacheRoundTrip() throws {
    let root = FileManager.default.temporaryDirectory
        .appendingPathComponent("LiveChatMediaTest-\(UUID().uuidString)", isDirectory: true)
    defer { try? FileManager.default.removeItem(at: root) }
    let cache = ImageMediaCache(diskRoot: root)
    let key = "media/u_9/x.png"
    let payload = Data("thumb-bytes".utf8)
    cache.store(payload, forKey: key)
    #expect(cache.data(forKey: key) == payload)
}

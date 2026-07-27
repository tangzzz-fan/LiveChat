import Foundation
import ChatDomain

/// 媒体上传 / 授权下载（对齐服务端 0014：initiate → PUT parts → complete → download/auth）。
public final class MediaAPI: MediaRepository, Sendable {
    private let http: HTTPClient
    private let session: SessionStore
    private let cache: ImageMediaCache

    public init(http: HTTPClient, session: SessionStore, cache: ImageMediaCache = .shared) {
        self.http = http
        self.session = session
        self.cache = cache
    }

    public func uploadImage(
        _ data: Data,
        metadata: ImageMetadata,
        conversationID: String
    ) async throws -> Attachment {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }

        struct InitiateBody: Encodable {
            let mime_type: String
            let size_bytes: Int64
            let file_name: String
            let width: Int
            let height: Int
            let conversation_id: String
        }
        struct InitiateResp: Decodable {
            let upload_id: String
            let object_key: String
            let chunk_size: Int
            let presigned_urls: [String]
            let expires_at_ms: Int64
        }

        let initiate: InitiateResp = try await http.postJSON(
            path: "/v1/media/upload/initiate",
            body: InitiateBody(
                mime_type: metadata.mimeType,
                size_bytes: metadata.sizeBytes,
                file_name: metadata.fileName,
                width: metadata.width,
                height: metadata.height,
                conversation_id: conversationID
            ),
            bearerToken: creds.accessToken
        )

        var parts: [(partNumber: Int, etag: String)] = []
        let chunkSize = max(initiate.chunk_size, 1)
        for (index, url) in initiate.presigned_urls.enumerated() {
            let start = index * chunkSize
            guard start < data.count else { break }
            let end = min(start + chunkSize, data.count)
            let slice = data.subdata(in: start..<end)
            try await http.putData(pathOrURL: url, data: slice, contentType: metadata.mimeType)
            parts.append((partNumber: index + 1, etag: "part_\(index + 1)"))
        }

        struct CompleteBody: Encodable {
            let upload_id: String
            let object_key: String
            let parts: [PartBody]
        }
        struct PartBody: Encodable {
            let part_number: Int
            let etag: String
        }
        struct CompleteResp: Decodable {
            let status: String
        }

        let _: CompleteResp = try await http.postJSON(
            path: "/v1/media/upload/\(initiate.upload_id)/complete",
            body: CompleteBody(
                upload_id: initiate.upload_id,
                object_key: initiate.object_key,
                parts: parts.map { PartBody(part_number: $0.partNumber, etag: $0.etag) }
            ),
            bearerToken: creds.accessToken
        )

        // 本地先缓存原图（展示时再降采样），避免立刻再打网。
        cache.store(data, forKey: initiate.object_key)

        return Attachment(
            objectKey: initiate.object_key,
            mimeType: metadata.mimeType,
            sizeBytes: metadata.sizeBytes,
            width: metadata.width,
            height: metadata.height
        )
    }

    public func downloadImage(objectKey: String, conversationID: String) async throws -> Data {
        if let cached = cache.data(forKey: objectKey) {
            return cached
        }
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }

        struct AuthBody: Encodable {
            let object_key: String
            let conversation_id: String
        }
        struct AuthResp: Decodable {
            let download_url: String
            let expires_in_sec: Int64
            let content_length: Int64
            let content_type: String
        }

        let auth: AuthResp = try await http.postJSON(
            path: "/v1/media/download/auth",
            body: AuthBody(object_key: objectKey, conversation_id: conversationID),
            bearerToken: creds.accessToken
        )
        let data = try await http.getData(pathOrURL: auth.download_url)
        cache.store(data, forKey: objectKey)
        return data
    }
}

/// 内存 + 磁盘双层缓存（薄实现，对齐高负载 #9）。
public final class ImageMediaCache: @unchecked Sendable {
    public static let shared = ImageMediaCache()

    private let memory = NSCache<NSString, NSData>()
    private let diskRoot: URL
    private let lock = NSLock()

    public init(diskRoot: URL? = nil) {
        self.diskRoot = diskRoot ?? FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("LiveChatMedia", isDirectory: true)
        try? FileManager.default.createDirectory(at: self.diskRoot, withIntermediateDirectories: true)
        memory.countLimit = 64
    }

    public func data(forKey key: String) -> Data? {
        let nsKey = key as NSString
        if let mem = memory.object(forKey: nsKey) {
            return mem as Data
        }
        lock.lock()
        defer { lock.unlock() }
        let url = diskURL(for: key)
        guard let data = try? Data(contentsOf: url) else { return nil }
        memory.setObject(data as NSData, forKey: nsKey)
        return data
    }

    public func store(_ data: Data, forKey key: String) {
        memory.setObject(data as NSData, forKey: key as NSString)
        lock.lock()
        defer { lock.unlock() }
        try? data.write(to: diskURL(for: key), options: .atomic)
    }

    private func diskURL(for key: String) -> URL {
        let safe = key.replacingOccurrences(of: "/", with: "_")
        return diskRoot.appendingPathComponent(safe)
    }
}

public enum ImageMessageContent {
    /// 服务端校验的 image content JSON。
    public static func encodeAttachment(_ attachment: Attachment) throws -> String {
        struct Payload: Encodable {
            struct Att: Encodable {
                let object_key: String
                let mime_type: String
                let size_bytes: Int64
                let width: Int?
                let height: Int?
            }
            let attachment: Att
        }
        let payload = Payload(
            attachment: .init(
                object_key: attachment.objectKey,
                mime_type: attachment.mimeType,
                size_bytes: attachment.sizeBytes,
                width: attachment.width,
                height: attachment.height
            )
        )
        let data = try JSONEncoder().encode(payload)
        return String(data: data, encoding: .utf8) ?? "{}"
    }

    public static func parseObjectKey(from content: String?) -> String? {
        guard let content, let data = content.data(using: .utf8) else { return nil }
        struct Payload: Decodable {
            struct Att: Decodable { let object_key: String? }
            let attachment: Att?
        }
        return try? JSONDecoder().decode(Payload.self, from: data).attachment?.object_key
    }
}

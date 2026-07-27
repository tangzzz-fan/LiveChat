import Foundation

public enum HTTPClientError: Error, Sendable, LocalizedError {
    case invalidResponse
    case status(code: Int, body: String)
    case rateLimited(retryAfter: TimeInterval, body: String)
    case decoding(Error)

    public var errorDescription: String? {
        switch self {
        case .invalidResponse:
            return "Invalid HTTP response"
        case .status(let code, let body):
            return "HTTP \(code): \(body)"
        case .rateLimited(let retryAfter, _):
            return "HTTP 429 retry after \(Int(retryAfter))s"
        case .decoding(let error):
            return "Decode failed: \(error.localizedDescription)"
        }
    }
}

public struct APIConfig: Sendable {
    public var baseURL: URL

    public init(baseURL: URL = URL(string: "http://127.0.0.1:8080")!) {
        self.baseURL = baseURL
    }
}

public final class HTTPClient: Sendable {
    private let session: URLSession
    public let config: APIConfig

    public init(config: APIConfig = APIConfig(), session: URLSession = .shared) {
        self.config = config
        self.session = session
    }

    public func postJSON<Body: Encodable, Response: Decodable>(
        path: String,
        body: Body,
        bearerToken: String? = nil
    ) async throws -> Response {
        var request = URLRequest(url: config.baseURL.appending(path: path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let bearerToken {
            request.setValue("Bearer \(bearerToken)", forHTTPHeaderField: "Authorization")
        }
        request.httpBody = try JSONEncoder().encode(body)
        return try await send(request)
    }

    public func getJSON<Response: Decodable>(
        path: String,
        bearerToken: String,
        query: [URLQueryItem] = []
    ) async throws -> Response {
        var components = URLComponents(
            url: config.baseURL.appending(path: path),
            resolvingAgainstBaseURL: false
        )!
        if !query.isEmpty {
            components.queryItems = query
        }
        var request = URLRequest(url: components.url!)
        request.httpMethod = "GET"
        request.setValue("Bearer \(bearerToken)", forHTTPHeaderField: "Authorization")
        return try await send(request)
    }

    /// PUT 原始字节（媒体分片上传）。`pathOrURL` 可为相对路径（可含 query）或绝对 URL。
    public func putData(
        pathOrURL: String,
        data: Data,
        contentType: String = "application/octet-stream"
    ) async throws {
        var request = URLRequest(url: resolveURL(pathOrURL))
        request.httpMethod = "PUT"
        request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = data
        let (_, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw HTTPClientError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            throw HTTPClientError.status(code: http.statusCode, body: "put failed")
        }
    }

    public func getData(pathOrURL: String, bearerToken: String? = nil) async throws -> Data {
        var request = URLRequest(url: resolveURL(pathOrURL))
        request.httpMethod = "GET"
        if let bearerToken {
            request.setValue("Bearer \(bearerToken)", forHTTPHeaderField: "Authorization")
        }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw HTTPClientError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? ""
            throw HTTPClientError.status(code: http.statusCode, body: body)
        }
        return data
    }

    private func resolveURL(_ pathOrURL: String) -> URL {
        if let absolute = URL(string: pathOrURL), absolute.scheme != nil {
            return absolute
        }
        return URL(string: pathOrURL, relativeTo: config.baseURL)!.absoluteURL
    }

    private func send<Response: Decodable>(_ request: URLRequest) async throws -> Response {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw HTTPClientError.invalidResponse
        }
        if http.statusCode == 429 {
            let body = String(data: data, encoding: .utf8) ?? ""
            let header = http.value(forHTTPHeaderField: "Retry-After").flatMap(TimeInterval.init)
            let fromJSON = (try? JSONDecoder().decode(RetryAfterBody.self, from: data))?.retry_after_sec
            let retryAfter = TimeInterval(fromJSON ?? Int(header ?? 5))
            throw HTTPClientError.rateLimited(retryAfter: retryAfter, body: body)
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? ""
            throw HTTPClientError.status(code: http.statusCode, body: body)
        }
        do {
            return try JSONDecoder().decode(Response.self, from: data)
        } catch {
            throw HTTPClientError.decoding(error)
        }
    }
}

private struct RetryAfterBody: Decodable {
    let retry_after_sec: Int?
}

private extension URL {
    func appending(path: String) -> URL {
        let trimmed = path.hasPrefix("/") ? String(path.dropFirst()) : path
        return appendingPathComponent(trimmed)
    }
}

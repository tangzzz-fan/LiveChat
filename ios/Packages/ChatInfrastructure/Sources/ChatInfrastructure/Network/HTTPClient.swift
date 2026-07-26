import Foundation

public struct APIConfig: Sendable {
    public var baseURL: URL

    public init(baseURL: URL = URL(string: "http://127.0.0.1:8080")!) {
        self.baseURL = baseURL
    }
}

public enum HTTPClientError: Error, Sendable, LocalizedError {
    case invalidResponse
    case status(code: Int, body: String)
    case decoding(Error)

    public var errorDescription: String? {
        switch self {
        case .invalidResponse:
            return "Invalid HTTP response"
        case .status(let code, let body):
            return "HTTP \(code): \(body)"
        case .decoding(let error):
            return "Decode failed: \(error.localizedDescription)"
        }
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
            request.setValue("Bearer \(bearerToken)", forHTTPHeaderField: Authorization)
        }
        request.httpBody = try JSONEncoder().encode(body)
        return try await send(request)
    }

    public func getJSON<Response: Decodable>(
        path: String,
        bearerToken: String
    ) async throws -> Response {
        var request = URLRequest(url: config.baseURL.appending(path: path))
        request.httpMethod = "GET"
        request.setValue("Bearer \(bearerToken)", forHTTPHeaderField: "Authorization")
        return try await send(request)
    }

    private let Authorization = "Authorization"

    private func send<Response: Decodable>(_ request: URLRequest) async throws -> Response {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw HTTPClientError.invalidResponse
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

private extension URL {
    func appending(path: String) -> URL {
        let trimmed = path.hasPrefix("/") ? String(path.dropFirst()) : path
        return appendingPathComponent(trimmed)
    }
}

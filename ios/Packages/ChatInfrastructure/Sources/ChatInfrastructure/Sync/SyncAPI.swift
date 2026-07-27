import Foundation
import ChatDomain

public final class SyncAPI: Sendable {
    private let http: HTTPClient
    private let session: SessionStore

    public init(http: HTTPClient, session: SessionStore) {
        self.http = http
        self.session = session
    }

    public func fetchEvents(from cursor: Int64, limit: Int = 50) async throws -> SyncResponse {
        guard let creds = try session.load() else {
            throw HTTPClientError.status(code: 401, body: "not logged in")
        }
        struct Resp: Decodable {
            let events: [SyncEvent]
            let has_more: Bool
            let latest_event_seq: Int64
        }
        let resp: Resp = try await http.getJSON(
            path: "/v1/sync/events",
            bearerToken: creds.accessToken,
            query: [
                URLQueryItem(name: "cursor", value: String(cursor)),
                URLQueryItem(name: "limit", value: String(limit)),
            ]
        )
        return SyncResponse(
            events: resp.events,
            hasMore: resp.has_more,
            latestEventSeq: resp.latest_event_seq
        )
    }
}

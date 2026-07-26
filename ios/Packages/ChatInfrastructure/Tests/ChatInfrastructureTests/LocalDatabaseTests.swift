import Testing
@testable import ChatInfrastructure

@Test
func inMemoryDatabaseMigrates() throws {
    let db = try LocalDatabase.inMemory()
    try db.dbQueue.read { database in
        let tables = try String.fetchAll(
            database,
            sql: "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
        )
        #expect(tables.contains("messages"))
        #expect(tables.contains("conversation_summaries"))
        #expect(tables.contains("sync_cursors"))
    }
}

@Test
func swiftProtobufLinks() {
    #expect(ProtobufScaffold.libraryLinked)
}

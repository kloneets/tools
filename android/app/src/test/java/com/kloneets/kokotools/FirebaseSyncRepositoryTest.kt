package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.json.JSONObject
import org.junit.Test
import java.net.URLDecoder
import java.time.OffsetDateTime
import java.time.ZoneOffset

class FirebaseSyncRepositoryTest {
    @Test
    fun backendConfiguredRejectsInvalidDatabaseUrls() {
        val base = FirebaseSettings(
            enabled = true,
            realtime = true,
            apiKey = "key",
            databaseUrl = "https://example.firebaseio.com",
            workspaceId = "workspace",
        )

        listOf("", "   ", "example.firebaseio.com", "http://example.firebaseio.com", "https://").forEach { invalid ->
            assertFalse(FirebaseSyncRepository.backendConfigReady(base.copy(databaseUrl = invalid)))
        }

        assertTrue(FirebaseSyncRepository.backendConfigReady(base))
    }

    @Test
    fun googleSignInPostBodyUsesGoogleProviderAndIdToken() {
        val postBody = FirebaseSyncRepository.googleSignInPostBody("token with spaces")
        val values = postBody
            .split("&")
            .associate {
                val parts = it.split("=", limit = 2)
                parts[0] to URLDecoder.decode(parts[1], Charsets.UTF_8.name())
            }

        assertEquals("token with spaces", values["id_token"])
        assertEquals("google.com", values["providerId"])
    }

    @Test
    fun parseRemoteNotesNormalizesAndSortsWithFallbackIdsAndTombstones() {
        val remote = JSONObject()
            .put("fallback-z", JSONObject()
                .put("id", "")
                .put("path", "Folder\\Z.md")
                .put("text", "")
                .put("rev", 3L)
                .put("deleted", true))
            .put("a", JSONObject()
                .put("id", "remote-a")
                .put("path", "folder/a.md")
                .put("text", "body")
                .put("rev", 2L))
            .put("invalid", JSONObject().put("text", "missing path"))

        val notes = FirebaseSyncRepository.parseRemoteNotes(remote)

        assertEquals(listOf("remote-a", "fallback-z"), notes.map { it.id })
        assertEquals(listOf("folder/a.md", "Folder/Z.md"), notes.map { it.path })
        assertTrue(notes.last().deleted)
    }

    @Test
    fun parseRemoteNotesPreservesTextRevisionAndTombstone() {
        val remote = JSONObject().put("note-key", JSONObject()
            .put("path", "note.md")
            .put("text", "note body")
            .put("rev", 7L)
            .put("deleted", true))
        val note = FirebaseSyncRepository.parseRemoteNotes(remote).single()

        assertEquals("note-key", note.id)
        assertEquals("note body", note.text)
        assertEquals(7L, note.rev)
        assertTrue(note.deleted)
    }

    @Test
    fun todoSyncFeatureKeysKeepDefaultLegacyNamesAndScopeNamedLists() {
        assertEquals("todos", FirebaseSyncRepository.todoFeature("default"))
        assertEquals("todo_archive_months", FirebaseSyncRepository.todoArchiveMonthsFeature(""))
        assertEquals("todo_archive_month:2026-05", FirebaseSyncRepository.todoArchiveMonthFeature("default", "2026-05"))

        assertEquals("todo_list:work:todos", FirebaseSyncRepository.todoFeature("Work"))
        assertEquals("todo_list:work:todo_archive_months", FirebaseSyncRepository.todoArchiveMonthsFeature("Work"))
        assertEquals("todo_list:work:todo_archive_month:2026-05", FirebaseSyncRepository.todoArchiveMonthFeature("Work", "2026-05"))
    }

    @Test
    fun todoDatabasePathsKeepDefaultLegacyBranchAndScopeNamedLists() {
        assertEquals("workspaces/ws/todos/t1", FirebaseSyncRepository.todoRecordPath("ws", "default", "t1"))
        assertEquals("workspaces/ws/todo_archive_months", FirebaseSyncRepository.todoArchiveMonthsPath("ws", "default"))
        assertEquals("workspaces/ws/todo_archives/2026-05", FirebaseSyncRepository.todoArchiveMonthPath("ws", "default", "2026-05"))

        assertEquals("workspaces/ws/todo_lists/work/todos/t1", FirebaseSyncRepository.todoRecordPath("ws", "work", "t1"))
        assertEquals("workspaces/ws/todo_lists/work/archive_months", FirebaseSyncRepository.todoArchiveMonthsPath("ws", "work"))
        assertEquals("workspaces/ws/todo_lists/work/archives/2026-05", FirebaseSyncRepository.todoArchiveMonthPath("ws", "work", "2026-05"))
        assertEquals("workspaces/ws/todo_lists/work/meta", FirebaseSyncRepository.todoListMetaPath("ws", "work"))
    }

    @Test
    fun todoListMetadataWriteAllowsOnlyMonotonicOrDeleteTieUpdates() {
        val now = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val remote = todoListMeta("work", "Work", now, rev = 10L)
        val deletedRemote = remote.copy(deleted = true, deletedAt = now)

        assertTrue(FirebaseSyncRepository.shouldWriteTodoListMeta(remote.copy(rev = 11L), remote))
        assertFalse(FirebaseSyncRepository.shouldWriteTodoListMeta(remote.copy(rev = 9L), remote))
        assertFalse(FirebaseSyncRepository.shouldWriteTodoListMeta(remote.copy(name = "Renamed"), remote))
        assertTrue(FirebaseSyncRepository.shouldWriteTodoListMeta(remote, remote))
        assertTrue(
            FirebaseSyncRepository.shouldWriteTodoListMeta(
                remote.copy(deleted = true, deletedAt = now, updatedAt = now.minusMinutes(1)),
                remote,
            ),
        )
        assertFalse(FirebaseSyncRepository.shouldWriteTodoListMeta(remote, deletedRemote))
    }

    @Test
    fun todoMergePreservesDefaultLocalOnlyItemsWhenRemoteIsEmpty() {
        val now = OffsetDateTime.of(2026, 9, 5, 10, 0, 0, 0, ZoneOffset.UTC)
        val local = TodoStore(items = listOf(todoItem("default-local", now)))

        val got = FirebaseSyncRepository.mergeTodoRecords(local, emptyList(), replaceLocal = false)

        assertEquals(listOf("default-local"), got.items.map { it.id })
    }

    @Test
    fun todoMergePreservesNamedListLocalOnlyItemsWhenRemoteIsEmpty() {
        val now = OffsetDateTime.of(2026, 9, 5, 10, 0, 0, 0, ZoneOffset.UTC)
        val local = TodoStore(items = listOf(todoItem("named-local", now)))

        val got = FirebaseSyncRepository.mergeTodoRecords(local, emptyList(), replaceLocal = false)

        assertEquals(listOf("named-local"), got.items.map { it.id })
    }

    @Test
    fun todoMergeExplicitReplaceDropsLocalOnlyItems() {
        val now = OffsetDateTime.of(2026, 9, 5, 10, 0, 0, 0, ZoneOffset.UTC)
        val local = TodoStore(
            items = listOf(todoItem("local-only", now)),
            archiveMonths = listOf("2026-09"),
        )

        val got = FirebaseSyncRepository.mergeTodoRecords(local, emptyList(), replaceLocal = true)

        assertEquals(emptyList<String>(), got.items.map { it.id })
        assertEquals(emptyList<String>(), got.archiveMonths)
    }

    @Test
    fun todoMergeExplicitReplaceKeepsRemoteItems() {
        val now = OffsetDateTime.of(2026, 9, 5, 10, 0, 0, 0, ZoneOffset.UTC)
        val local = TodoStore(items = listOf(todoItem("local-only", now)))
        val remote = listOf(
            FirebaseRemoteTodo(
                item = todoItem("remote", now.plusMinutes(1)),
                rev = now.plusMinutes(1).toInstant().toEpochMilli(),
                deleted = false,
            ),
        )

        val got = FirebaseSyncRepository.mergeTodoRecords(local, remote, replaceLocal = true)

        assertEquals(listOf("remote"), got.items.map { it.id })
    }

    @Test
    fun noteWriterPreservesCurrentRecordBeforeReplacingIt() {
        val events = mutableListOf<String>()
        val writes = linkedMapOf<String, JSONObject>()
        val current = JSONObject()
            .put("id", "note-id")
            .put("path", "note.md")
            .put("text", "before")
            .put("rev", 7L)
            .put("deleted", false)
        val replacement = JSONObject()
            .put("id", "note-id")
            .put("path", "note.md")
            .put("text", "after")
            .put("rev", 8L)
            .put("deleted", false)
        val writer = FirebaseNoteHistoryWriter(
            getRecord = { path ->
                events += "GET $path"
                FirebaseVersionedNote(current, "note-etag-7")
            },
            putHistory = { path, body ->
                events += "PUT $path"
                writes[path] = JSONObject(body.toString())
            },
            putCurrent = { path, body, etag ->
                events += "PUT $path $etag"
                writes[path] = JSONObject(body.toString())
            },
        )

        writer.replace("workspaces/workspace", "note-id", replacement)

        assertEquals(
            listOf(
                "GET workspaces/workspace/notes/note-id",
                "PUT workspaces/workspace/note_versions/note-id/7",
                "PUT workspaces/workspace/notes/note-id note-etag-7",
            ),
            events,
        )
        assertEquals("before", writes.getValue("workspaces/workspace/note_versions/note-id/7").getString("text"))
        assertEquals(7L, writes.getValue("workspaces/workspace/note_versions/note-id/7").getLong("rev"))
        assertEquals("after", writes.getValue("workspaces/workspace/notes/note-id").getString("text"))
    }

    @Test
    fun noteWriterAbortsLiveOverwriteWhenHistoryWriteFails() {
        val events = mutableListOf<String>()
        val current = JSONObject().put("path", "note.md").put("text", "before").put("rev", 4L)
        val writer = FirebaseNoteHistoryWriter(
            getRecord = { path ->
                events += "GET $path"
                FirebaseVersionedNote(current, "note-etag-4")
            },
            putHistory = { path, _ ->
                events += "PUT $path"
                if (path.contains("/note_versions/")) throw IllegalStateException("history unavailable")
            },
            putCurrent = { path, _, _ -> events += "PUT $path" },
        )

        assertThrows(IllegalStateException::class.java) {
            writer.replace("workspaces/workspace", "note-id", JSONObject().put("rev", 5L))
        }

        assertEquals(
            listOf(
                "GET workspaces/workspace/notes/note-id",
                "PUT workspaces/workspace/note_versions/note-id/4",
            ),
            events,
        )
    }

    @Test
    fun noteWriterPropagatesStaleConditionalWriteFailure() {
        val events = mutableListOf<String>()
        val current = JSONObject().put("path", "note.md").put("text", "before").put("rev", 4L)
        val writer = FirebaseNoteHistoryWriter(
            getRecord = { path ->
                events += "GET $path"
                FirebaseVersionedNote(current, "note-etag-4")
            },
            putHistory = { path, _ -> events += "PUT $path" },
            putCurrent = { path, _, etag ->
                events += "PUT $path $etag"
                throw IllegalStateException("precondition failed")
            },
        )

        assertThrows(IllegalStateException::class.java) {
            writer.replace("workspaces/workspace", "note-id", JSONObject().put("rev", 5L))
        }

        assertEquals(
            listOf(
                "GET workspaces/workspace/notes/note-id",
                "PUT workspaces/workspace/note_versions/note-id/4",
                "PUT workspaces/workspace/notes/note-id note-etag-4",
            ),
            events,
        )
    }

    @Test
    fun noteWriterCreatesNewRecordWithoutHistoryWhenRemoteIsMissing() {
        val events = mutableListOf<String>()
        val writer = FirebaseNoteHistoryWriter(
            getRecord = { path ->
                events += "GET $path"
                FirebaseVersionedNote(null, "null-etag")
            },
            putHistory = { path, _ -> events += "PUT $path" },
            putCurrent = { path, _, etag -> events += "PUT $path $etag" },
        )

        writer.replace("workspaces/workspace", "note-id", JSONObject().put("rev", 1L))

        assertEquals(
            listOf(
                "GET workspaces/workspace/notes/note-id",
                "PUT workspaces/workspace/notes/note-id null-etag",
            ),
            events,
        )
    }

    @Test
    fun noteWriterAdvancesRevisionPastCurrentRemoteRevision() {
        var written: JSONObject? = null
        val writer = FirebaseNoteHistoryWriter(
            getRecord = { FirebaseVersionedNote(JSONObject().put("rev", 10L), "etag") },
            putHistory = { _, _ -> },
            putCurrent = { path, body, _ ->
                if (path.endsWith("/notes/note-id")) written = JSONObject(body.toString())
            },
        )

        writer.replace("workspaces/workspace", "note-id", JSONObject().put("rev", 10L))

        assertEquals(11L, requireNotNull(written).getLong("rev"))
    }

    @Test
    fun missingFirebaseNoteEtagFailsBeforeAnyWrite() {
        var writes = 0
        val writer = FirebaseNoteHistoryWriter(
            getRecord = {
                FirebaseVersionedNote(
                    record = null,
                    etag = requireFirebaseNoteEtag(null),
                )
            },
            putHistory = { _, _ -> writes++ },
            putCurrent = { _, _, _ -> writes++ },
        )

        assertThrows(IllegalStateException::class.java) {
            writer.replace("workspaces/workspace", "note-id", JSONObject().put("rev", 1L))
        }

        assertEquals(0, writes)
    }

    @Test
    fun blankFirebaseNoteEtagIsRejectedByWriterContract() {
        assertThrows(IllegalArgumentException::class.java) {
            FirebaseVersionedNote(record = null, etag = " ")
        }
    }

    @Test
    fun todoStoreHashIgnoresArchivedItemBodies() {
        val now = OffsetDateTime.of(2026, 6, 1, 0, 0, 0, 0, ZoneOffset.UTC)
        val store = TodoStore(items = listOf(
            TodoItem(id = "active", text = "active", status = TodoRepository.STATUS_TODO, order = 0, createdAt = now, updatedAt = now),
            TodoItem(id = "archived", text = "large archived body", status = TodoRepository.STATUS_ARCHIVED, order = 1, createdAt = now, updatedAt = now, archivedAt = now),
        ))
        val changedArchiveBody = store.copy(items = store.items.map {
            if (it.id == "archived") it.copy(text = "changed") else it
        })

        assertEquals(
            FirebaseSyncRepository.todoStoreHash(store),
            FirebaseSyncRepository.todoStoreHash(changedArchiveBody),
        )
    }

    @Test
    fun goldenSyncHashesMatchSharedFixture() {
        val fixture = JSONObject(readResource("golden_sync_fixture.json"))
        val expected = fixture.getJSONObject("expected")
        val store = TodoStore(
            items = fixture.getJSONObject("todo_store")
                .getJSONArray("items")
                .let { items ->
                    (0 until items.length()).map { index ->
                        TodoRepository.parseItem(items.getJSONObject(index))
                    }
                },
            archiveMonths = fixture.getJSONObject("todo_store")
                .getJSONArray("archive_months")
                .let { months -> (0 until months.length()).map { months.getString(it) } },
        )
        assertEquals(expected.getString("todo_store_hash"), FirebaseSyncRepository.todoStoreHash(store))
        assertEquals(expected.getString("todo_archive_months_hash"), FirebaseSyncRepository.todoArchiveMonthsHash(store.archiveMonths))
        assertEquals(expected.getString("shared_settings_hash"), FirebaseSyncRepository.sharedSettingsHash(fixture.getJSONObject("shared_settings")))
    }

    private fun readResource(name: String): String {
        return requireNotNull(javaClass.classLoader?.getResource(name)) { "missing resource $name" }.readText()
    }

    private fun todoListMeta(
        id: String,
        name: String,
        now: OffsetDateTime,
        rev: Long,
    ): TodoListMeta {
        return TodoListMeta(
            id = id,
            name = name,
            rev = rev,
            createdAt = now,
            updatedAt = now,
        )
    }

    private fun todoItem(id: String, now: OffsetDateTime): TodoItem {
        return TodoItem(
            id = id,
            text = id,
            status = TodoRepository.STATUS_TODO,
            order = 0,
            createdAt = now,
            updatedAt = now,
        )
    }
}

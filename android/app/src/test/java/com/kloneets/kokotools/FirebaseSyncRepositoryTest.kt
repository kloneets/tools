package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test
import java.net.URLDecoder
import java.time.OffsetDateTime
import java.time.ZoneOffset

class FirebaseSyncRepositoryTest {
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
    fun noteMetadataHashIsStableForInputOrder() {
        val notes = listOf(
            FirebaseRemoteNote(id = "b", path = "B.md", text = "body two", rev = 2L, deleted = false),
            FirebaseRemoteNote(id = "a", path = "A.md", text = "body one", rev = 1L, deleted = false),
        )

        assertEquals(
            FirebaseSyncRepository.noteMetadataHash(notes),
            FirebaseSyncRepository.noteMetadataHash(notes.reversed()),
        )
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
}

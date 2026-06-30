package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import java.time.OffsetDateTime
import java.time.ZoneOffset

class TodoRepositoryTest {
    @Test
    fun nonArchivedStoreFiltersArchivedItems() {
        val now = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val store = TodoStore(
            items = listOf(
                todoItem("active", TodoRepository.STATUS_TODO, now),
                todoItem("old", TodoRepository.STATUS_ARCHIVED, now, archivedAt = now),
            ),
        )

        val got = TodoRepository.nonArchivedStore(store)

        assertEquals(listOf("active"), got.items.map { it.id })
    }

    @Test
    fun archiveMonthsIncludesCachedMonthTitlesWithoutItems() {
        val store = TodoStore(archiveMonths = listOf("2026-05"))

        assertEquals(listOf("2026-05"), TodoRepository.archiveMonths(store))
    }

    @Test
    fun mergeArchiveMonthReplacesOnlyRequestedMonth() {
        val may = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val april = OffsetDateTime.of(2026, 4, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val store = TodoStore(
            items = listOf(
                todoItem("active", TodoRepository.STATUS_TODO, may),
                todoItem("old-may", TodoRepository.STATUS_ARCHIVED, may, archivedAt = may),
                todoItem("old-april", TodoRepository.STATUS_ARCHIVED, april, archivedAt = april),
            ),
        )

        val got = TodoRepository.mergeArchiveMonth(
            store,
            "2026-05",
            listOf(todoItem("new-may", TodoRepository.STATUS_ARCHIVED, may, archivedAt = may)),
        )

        assertEquals(listOf("new-may"), TodoRepository.archiveMonthItems(got, "2026-05").map { it.id })
        assertEquals(listOf("old-april"), TodoRepository.archiveMonthItems(got, "2026-04").map { it.id })
        assertFalse(got.items.any { it.id == "old-may" })
    }

    @Test
    fun mergeArchiveMonthKeepsLocalCacheWhenRemoteMonthIsEmpty() {
        val may = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val store = TodoStore(
            items = listOf(
                todoItem("active", TodoRepository.STATUS_TODO, may),
                todoItem("old-may", TodoRepository.STATUS_ARCHIVED, may, archivedAt = may),
            ),
        )

        val got = TodoRepository.mergeArchiveMonth(store, "2026-05", emptyList())

        assertEquals(listOf("old-may"), TodoRepository.archiveMonthItems(got, "2026-05").map { it.id })
        assertEquals(listOf("2026-05"), TodoRepository.archiveMonths(got))
    }

    @Test
    fun mergeArchiveMonthRemembersTitleWhenRemoteMonthIsEmpty() {
        val got = TodoRepository.mergeArchiveMonth(TodoStore(), "2026-05", emptyList())

        assertEquals(listOf("2026-05"), TodoRepository.archiveMonths(got))
    }

    @Test
    fun preserveArchivedKeepsLocalArchiveCacheAfterSync() {
        val now = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val local = TodoStore(
            items = listOf(
                todoItem("local-active", TodoRepository.STATUS_TODO, now),
                todoItem("old", TodoRepository.STATUS_ARCHIVED, now, archivedAt = now),
            ),
        )
        val synced = TodoStore(items = listOf(todoItem("remote-active", TodoRepository.STATUS_TODO, now)))

        val got = TodoRepository.preserveArchived(local, synced)

        assertEquals(setOf("remote-active", "old"), got.items.map { it.id }.toSet())
    }

    @Test
    fun preserveArchivedDropsArchiveCacheWhenSyncedItemHasSameId() {
        val now = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)
        val local = TodoStore(items = listOf(todoItem("same", TodoRepository.STATUS_ARCHIVED, now, archivedAt = now)))
        val synced = TodoStore(items = listOf(todoItem("same", TodoRepository.STATUS_TODO, now)))

        val got = TodoRepository.preserveArchived(local, synced)

        assertEquals(listOf(TodoRepository.STATUS_TODO), got.items.map { it.status })
    }

    private fun todoItem(
        id: String,
        status: String,
        now: OffsetDateTime,
        archivedAt: OffsetDateTime? = null,
    ): TodoItem {
        return TodoItem(
            id = id,
            text = id,
            status = status,
            order = 0,
            createdAt = now,
            updatedAt = now,
            archivedAt = archivedAt,
        )
    }
}

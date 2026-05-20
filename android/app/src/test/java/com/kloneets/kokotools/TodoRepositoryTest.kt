package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.OffsetDateTime
import java.time.ZoneOffset

class TodoRepositoryTest {
    private val now: OffsetDateTime = OffsetDateTime.of(2026, 5, 20, 10, 0, 0, 0, ZoneOffset.UTC)

    @Test
    fun checkedTodoStaysActiveBeforeDelay() {
        val store = TodoStore(items = listOf(item("1", checkedAt = now)))

        val cleaned = TodoRepository.cleanup(store, now.plusSeconds(9))

        assertEquals(TodoRepository.STATUS_TODO, cleaned.items.single().status)
        assertEquals("1", TodoRepository.activeItems(cleaned).single().id)
    }

    @Test
    fun checkedTodoPromotesToDoneAfterDelay() {
        val store = TodoStore(items = listOf(item("1", checkedAt = now)))

        val cleaned = TodoRepository.cleanup(store, now.plusSeconds(10))

        assertEquals(TodoRepository.STATUS_DONE, cleaned.items.single().status)
        assertNotNull(cleaned.items.single().doneAt)
    }

    @Test
    fun doneTodoArchivesAfterSevenDays() {
        val doneAt = now.minusDays(7)
        val store = TodoStore(items = listOf(item("1", status = TodoRepository.STATUS_DONE, doneAt = doneAt)))

        val cleaned = TodoRepository.cleanup(store, now)

        assertEquals(TodoRepository.STATUS_ARCHIVED, cleaned.items.single().status)
        assertNotNull(cleaned.items.single().archivedAt)
    }

    @Test
    fun archiveGroupsByMonth() {
        val store = TodoStore(
            items = listOf(
                item("1", status = TodoRepository.STATUS_ARCHIVED, archivedAt = now),
                item("2", status = TodoRepository.STATUS_ARCHIVED, archivedAt = now.minusMonths(1)),
            ),
        )

        val groups = TodoRepository.archiveGroups(store)

        assertEquals(listOf("2026-05", "2026-04"), groups.keys.toList())
        assertEquals("1", groups.getValue("2026-05").single().id)
    }

    @Test
    fun activeItemsSortUncheckedBeforePendingChecked() {
        val store = TodoStore(
            items = listOf(
                item("checked", order = 0, checkedAt = now),
                item("active", order = 1),
            ),
        )

        val active = TodoRepository.activeItems(store)

        assertEquals(listOf("active", "checked"), active.map { it.id })
        assertNull(active.first().checkedAt)
    }

    @Test
    fun reorderActiveUncheckedMovesDraggedItemToTargetPosition() {
        val store = TodoStore(
            items = listOf(
                item("a", order = 0),
                item("b", order = 1),
                item("c", order = 2),
            ),
        )

        val reordered = TodoRepository.reorderActiveUnchecked(store, "c", "a", now.plusMinutes(1))

        assertEquals(listOf("c", "a", "b"), TodoRepository.activeItems(reordered).map { it.id })
    }

    @Test
    fun reorderActiveUncheckedIgnoresCheckedDoneAndArchivedItems() {
        val store = TodoStore(
            items = listOf(
                item("a", order = 0),
                item("checked", order = 1, checkedAt = now),
                item("done", status = TodoRepository.STATUS_DONE, order = 2, doneAt = now),
                item("archived", status = TodoRepository.STATUS_ARCHIVED, order = 3, archivedAt = now),
            ),
        )

        val checkedResult = TodoRepository.reorderActiveUnchecked(store, "checked", "a", now.plusMinutes(1))
        val doneResult = TodoRepository.reorderActiveUnchecked(store, "done", "a", now.plusMinutes(1))
        val archivedResult = TodoRepository.reorderActiveUnchecked(store, "archived", "a", now.plusMinutes(1))

        assertEquals(store, checkedResult)
        assertEquals(store, doneResult)
        assertEquals(store, archivedResult)
    }

    private fun item(
        id: String,
        status: String = TodoRepository.STATUS_TODO,
        order: Int = 0,
        checkedAt: OffsetDateTime? = null,
        doneAt: OffsetDateTime? = null,
        archivedAt: OffsetDateTime? = null,
    ): TodoItem {
        return TodoItem(
            id = id,
            text = id,
            status = status,
            order = order,
            createdAt = now,
            updatedAt = now,
            checkedAt = checkedAt,
            doneAt = doneAt,
            archivedAt = archivedAt,
        )
    }
}

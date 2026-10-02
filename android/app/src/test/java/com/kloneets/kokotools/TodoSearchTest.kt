package com.kloneets.kokotools

import org.junit.Assert.*
import org.junit.Test
import java.time.OffsetDateTime

class TodoSearchTest {
    @Test fun orderedUnicodeMatchingUsesEarliestUtf16Positions() {
        val cases = listOf(
            Triple("groceries", "grc", listOf(0, 1, 3)),
            Triple("banana", "aa", listOf(1, 3)),
            Triple("abc", "", emptyList()),
            Triple("abc", "ca", null),
            Triple("a.b", ".", listOf(1)),
            Triple("😀ĀΣςKı", "😀āσσki", listOf(0, 2, 3, 4, 5, 6)),
            Triple("aaa", "aaaa", null),
        )
        cases.forEach { (text, query, expected) -> assertEquals("$text / $query", expected, fuzzyTodoMatch(text, query)) }
    }

    private fun item(id: String, archived: Boolean = false) = TodoItem(
        id, "groceries $id", if (archived) TodoRepository.STATUS_ARCHIVED else TodoRepository.STATUS_TODO,
        0, OffsetDateTime.parse("2026-01-01T00:00:00Z"), OffsetDateTime.parse("2026-01-01T00:00:00Z"),
        archivedAt = if (archived) OffsetDateTime.parse("2026-01-01T00:00:00Z") else null,
    )

    @Test fun loadedArchiveWraparoundUpdatesAndListIsolation() {
        val search = TodoSearch()
        search.selectList("one")
        search.open = true
        search.query = "grc"
        var store = TodoStore(items = listOf(item("active"), item("archive", true)), archiveMonths = listOf("2025-12"))
        search.refresh(store)
        assertEquals(listOf("active", "archive"), search.results)
        search.move(-1)
        assertEquals("archive", search.selectedId)
        search.move(1)
        assertEquals("active", search.selectedId)
        search.move(1)
        store = store.copy(items = store.items.reversed())
        search.refresh(store)
        assertEquals("archive", search.selectedId)
        search.refresh(store.copy(items = listOf(item("active"))))
        assertEquals("active", search.selectedId)
        search.query = "missing"
        search.refresh(store)
        assertEquals("No matches", search.counter())
        search.selectList("one")
        assertEquals("missing", search.query)
        search.selectList("two")
        assertFalse(search.open)
        assertEquals("", search.query)
        assertNull(search.selectedId)
    }
}

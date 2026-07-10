package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.OffsetDateTime
import java.time.ZoneOffset

class TodoScreenRowsTest {
    @Test
    fun activeSectionsReturnShortThenLong() {
        val now = OffsetDateTime.of(2026, 6, 1, 0, 0, 0, 0, ZoneOffset.UTC)
        val store = TodoStore(items = listOf(
            TodoItem(id = "long", text = "long", status = TodoRepository.STATUS_TODO, term = TodoRepository.TERM_LONG, order = 0, createdAt = now, updatedAt = now),
            TodoItem(id = "short", text = "short", status = TodoRepository.STATUS_TODO, term = TodoRepository.TERM_SHORT, order = 0, createdAt = now, updatedAt = now),
        ))

        val sections = TodoScreenRows.activeSections(store)

        assertEquals(listOf("Short term", "Long term"), sections.map { it.title })
        assertEquals(listOf("short"), sections[0].items.map { it.id })
        assertEquals(listOf("long"), sections[1].items.map { it.id })
    }
}

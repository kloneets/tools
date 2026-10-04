package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class MultiCursorEditingTest {
    @Test
    fun addsNextExactOccurrenceAndStopsAtEnd() {
        val editor = MultiCursorEditing()
        editor.begin(0, 3)
        assertEquals(MultiCursorRange(8, 11), editor.addNext("cat dog cat cat", 0, 3))
        assertEquals(1, editor.primaryIndex)
        assertEquals(MultiCursorRange(12, 15), editor.addNext("cat dog cat cat", 8, 11))
        assertNull(editor.addNext("cat dog cat cat", 12, 15))
    }

    @Test
    fun usesUtf16OffsetsAndCanSelectWordAtCursor() {
        val editor = MultiCursorEditing()
        editor.begin(2, 5)
        assertEquals(MultiCursorRange(8, 11), editor.addNext("😀cat x cat", 2, 5))
    }

    @Test
    fun enteringAtCaretSelectsWordWithoutAddingSecondaryCursor() {
        val editor = MultiCursorEditing()
        editor.beginAtText("cat cat", 1, 1)

        assertEquals(listOf(MultiCursorRange(0, 3)), editor.ranges)
        assertEquals(MultiCursorRange(4, 7), editor.addNext("cat cat", 0, 3))
        assertEquals(2, editor.ranges.size)
    }

    @Test
    fun mirroredReplacementKeepsRangesOrderedAndDeduplicates() {
        val editor = MultiCursorEditing()
        editor.begin(0, 2)
        editor.addNext("go go", 0, 2)
        assertEquals("yes yes", editor.replaceAll("go go", "yes"))
        assertEquals(listOf(MultiCursorRange(0, 3), MultiCursorRange(4, 7)), editor.ranges)
        assertTrue(editor.primaryIndex in editor.ranges.indices)
    }

    @Test
    fun refusesOverlappingOccurrence() {
        val editor = MultiCursorEditing()
        editor.begin(0, 3)
        assertNull(editor.addNext("aaaaa", 0, 3))
    }
}

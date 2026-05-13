package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class NotesRepositoryTest {
    @Test
    fun normalizePathAppendsMarkdownExtension() {
        assertEquals("daily/today.md", NotesRepository.normalizePath("daily/today"))
    }

    @Test
    fun normalizePathRemovesUnsafeSegments() {
        assertEquals("nested/note.md", NotesRepository.normalizePath("../nested/./note.md"))
    }

    @Test
    fun normalizePathDefaultsBlankNames() {
        assertEquals("untitled.md", NotesRepository.normalizePath(" "))
    }
}

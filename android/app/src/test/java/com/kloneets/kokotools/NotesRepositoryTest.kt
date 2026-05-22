package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
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

    @Test
    fun normalizeAssetPathRemovesUnsafeSegmentsWithoutAddingMarkdownExtension() {
        assertEquals("daily.assets/photo.png", NotesRepository.normalizeAssetPath("../daily.assets/./photo.png"))
    }

    @Test
    fun detectsManagedAssetPaths() {
        assertTrue(NotesRepository.isManagedAssetPath("daily.assets/photo.png"))
        assertTrue(NotesRepository.isManagedAssetPath("assets/logo.png"))
        assertFalse(NotesRepository.isManagedAssetPath("daily/photo.png"))
    }

    @Test
    fun managedAssetPathUsesCurrentNoteWhenAvailable() {
        assertEquals("daily/today.assets/photo.png", NotesRepository.managedAssetPathForNote("daily/today.md", "../photo.png"))
        assertEquals("assets/photo.png", NotesRepository.managedAssetPathForNote("", "photo.png"))
    }
}

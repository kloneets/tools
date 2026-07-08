package com.kloneets.kokotools

import org.junit.Assert.assertEquals
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
    fun collectFoldersIncludesNoteAndDirectoryFolders() {
        val folders = NotesRepository.collectFolders(
            notePaths = listOf("daily/today.md", "Work/Ideas/one.md", "root.md"),
            directoryPaths = listOf("Archive/2026", "daily"),
        )

        assertEquals(listOf("Archive", "Archive/2026", "daily", "Work", "Work/Ideas"), folders)
    }

    @Test
    fun buildNewNotePathCombinesSelectedFolderAndName() {
        assertEquals("Work/today.md", NotesRepository.buildNewNotePath("Work", "today"))
    }

    @Test
    fun buildNewNotePathPreservesSlashFolderCreation() {
        assertEquals("Work/Ideas/today.md", NotesRepository.buildNewNotePath("Work", "Ideas/today"))
    }

    @Test
    fun buildNewNotePathRootKeepsExistingSlashBehavior() {
        assertEquals("daily/today.md", NotesRepository.buildNewNotePath("", "daily/today"))
    }

    @Test
    fun detectsManagedAssetDirectoryNames() {
        assertTrue(NotesRepository.isManagedAssetDirName("daily.assets"))
        assertTrue(NotesRepository.isManagedAssetDirName("assets"))
    }
}

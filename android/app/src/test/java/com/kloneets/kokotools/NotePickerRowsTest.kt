package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NotePickerRowsTest {
    @Test
    fun flatRowsUseFullRelativePaths() {
        val rows = NotePickerRows.flat(
            listOf(
                NoteFile("daily/today.md"),
                NoteFile("root.md"),
            ),
        )

        assertEquals(listOf("daily/today.md", "root.md"), rows.map { it.label })
        assertTrue(rows.none { it.folder })
    }

    @Test
    fun treeRowsCollapseFoldersByDefault() {
        val rows = NotePickerRows.tree(
            listOf(
                NoteFile("daily/today.md"),
                NoteFile("root.md"),
            ),
            expandedFolders = emptySet(),
        )

        assertEquals(listOf("[+] daily", "root.md"), rows.map { it.label })
        assertTrue(rows.first().folder)
        assertFalse(rows.last().folder)
    }

    @Test
    fun treeRowsRevealExpandedFolderChildren() {
        val rows = NotePickerRows.tree(
            listOf(
                NoteFile("daily/archive/old.md"),
                NoteFile("daily/today.md"),
                NoteFile("root.md"),
            ),
            expandedFolders = setOf("daily", "daily/archive"),
        )

        assertEquals(
            listOf("[-] daily", "[-] archive", "old.md", "today.md", "root.md"),
            rows.map { it.label },
        )
        assertEquals(listOf(0, 1, 2, 1, 0), rows.map { it.depth })
    }
}

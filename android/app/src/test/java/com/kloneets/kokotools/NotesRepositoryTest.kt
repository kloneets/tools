package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class NotesRepositoryTest {
    @get:Rule
    val temporaryFolder = TemporaryFolder()

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

    @Test
    fun overwriteCreatesReadableVersionOutsideLiveNotesTree() {
        val root = temporaryFolder.newFolder("files")
        val repository = NotesRepository(root, clock = { 100L })
        repository.save("Home/entry.md", "before")

        repository.save("Home/entry.md", "after")

        val version = repository.listVersions("Home/entry.md").single()
        assertEquals("Home/entry.md", version.relativePath)
        assertEquals(100L, version.createdAtMillis)
        assertEquals("before", repository.readVersion("Home/entry.md", version.versionId))
        assertEquals("after", repository.read("Home/entry.md"))
        assertTrue(File(root, NotesRepository.NOTE_BACKUPS_DIR).isDirectory)
        assertFalse(repository.notesPath().canonicalPath.startsWith(File(root, NotesRepository.NOTE_BACKUPS_DIR).canonicalPath))
        assertEquals(listOf("Home/entry.md"), repository.listNotes().map { it.relativePath })
    }

    @Test
    fun unchangedSaveDoesNotCreateRedundantVersion() {
        val repository = NotesRepository(temporaryFolder.newFolder("files"))
        repository.save("entry.md", "same")

        repository.save("entry.md", "same")

        assertTrue(repository.listVersions("entry.md").isEmpty())
    }

    @Test
    fun deleteBacksUpExistingNoteBeforeRemovingIt() {
        val repository = NotesRepository(temporaryFolder.newFolder("files"), clock = { 200L })
        repository.save("entry.md", "recover me")

        assertTrue(repository.delete("entry.md"))

        assertEquals("", repository.read("entry.md"))
        val version = repository.listVersions("entry.md").single()
        assertEquals("recover me", repository.readVersion("entry.md", version.versionId))
    }

    @Test
    fun clearAllBacksUpEveryNoteBeforeClearingLiveTree() {
        var now = 300L
        val repository = NotesRepository(temporaryFolder.newFolder("files"), clock = { now++ })
        repository.save("a.md", "alpha")
        repository.save("nested/b.md", "beta")

        repository.clearAll()

        assertTrue(repository.listNotes().isEmpty())
        val aVersion = repository.listVersions("a.md").single()
        val bVersion = repository.listVersions("nested/b.md").single()
        assertEquals("alpha", repository.readVersion("a.md", aVersion.versionId))
        assertEquals("beta", repository.readVersion("nested/b.md", bVersion.versionId))
    }

    @Test
    fun managedAssetCleanupBacksUpMarkdownFilesBeforeDeletingThem() {
        val repository = NotesRepository(temporaryFolder.newFolder("files"), clock = { 350L })
        val legacyMarkdown = File(repository.notesPath(), "entry.assets/context.md")
        legacyMarkdown.parentFile?.mkdirs()
        legacyMarkdown.writeText("legacy markdown")

        repository.cleanupManagedAssetDirs()

        assertFalse(legacyMarkdown.exists())
        val version = repository.listVersions("entry.assets/context.md").single()
        assertEquals("legacy markdown", repository.readVersion("entry.assets/context.md", version.versionId))
    }

    @Test
    fun restoreVersionBacksUpCurrentTextAndRestoresSelectedText() {
        var now = 400L
        val repository = NotesRepository(temporaryFolder.newFolder("files"), clock = { now++ })
        repository.save("entry.md", "first")
        repository.save("entry.md", "second")
        val firstVersion = repository.listVersions("entry.md").single()

        repository.restoreVersion("entry.md", firstVersion.versionId)

        assertEquals("first", repository.read("entry.md"))
        assertEquals(
            setOf("first", "second"),
            repository.listVersions("entry.md")
                .map { repository.readVersion("entry.md", it.versionId) }
                .toSet(),
        )
    }

    @Test
    fun backupRetentionIsBoundedPerNoteAndGlobally() {
        var now = 500L
        val repository = NotesRepository(
            filesDir = temporaryFolder.newFolder("files"),
            clock = { now++ },
            maxVersionsPerNote = 2,
            maxVersionsTotal = 3,
        )
        repository.save("a.md", "a0")
        repository.save("a.md", "a1")
        repository.save("a.md", "a2")
        repository.save("a.md", "a3")
        repository.save("b.md", "b0")
        repository.save("b.md", "b1")
        repository.save("b.md", "b2")

        assertTrue(repository.listVersions("a.md").size <= 2)
        assertTrue(repository.listVersions("b.md").size <= 2)
        assertEquals(3, repository.listVersions("a.md").size + repository.listVersions("b.md").size)
    }

    @Test
    fun invalidVersionIdCannotEscapeBackupDirectory() {
        val repository = NotesRepository(temporaryFolder.newFolder("files"))

        assertThrows(IllegalArgumentException::class.java) {
            repository.readVersion("entry.md", "../other.bak")
        }
    }

    @Test
    fun backupFailureAbortsOverwriteAndPreservesLiveText() {
        val root = temporaryFolder.newFolder("files")
        val repository = NotesRepository(root)
        repository.save("entry.md", "original")
        File(root, NotesRepository.NOTE_BACKUPS_DIR).writeText("blocks backup directory")

        assertThrows(IllegalStateException::class.java) {
            repository.save("entry.md", "replacement")
        }

        assertEquals("original", repository.read("entry.md"))
    }

    @Test
    fun backupFailureAbortsDeleteAndPreservesLiveText() {
        val root = temporaryFolder.newFolder("delete-files")
        val repository = NotesRepository(root)
        repository.save("entry.md", "original")
        File(root, NotesRepository.NOTE_BACKUPS_DIR).writeText("blocks backup directory")

        assertThrows(IllegalStateException::class.java) {
            repository.delete("entry.md")
        }

        assertEquals("original", repository.read("entry.md"))
    }

    @Test
    fun filesystemDeleteFailureThrowsAndPreservesLiveText() {
        val root = temporaryFolder.newFolder("failed-delete-files")
        val repository = NotesRepository(root, deleteFile = { false })
        repository.save("entry.md", "original")

        assertThrows(IllegalStateException::class.java) {
            repository.delete("entry.md")
        }

        assertEquals("original", repository.read("entry.md"))
        assertEquals("original", repository.readVersion("entry.md", repository.listVersions("entry.md").single().versionId))
    }

    @Test
    fun postDeleteActionsRunOnlyAfterConfirmedLocalDelete() {
        var postDeleteCalls = 0

        val deleted = completeLocalNoteDeletion(
            deleteLocal = { false },
            afterDelete = { postDeleteCalls++ },
        )

        assertFalse(deleted)
        assertEquals(0, postDeleteCalls)
    }

    @Test
    fun localDeleteExceptionSuppressesPostDeleteActions() {
        var postDeleteCalls = 0

        assertThrows(IllegalStateException::class.java) {
            completeLocalNoteDeletion(
                deleteLocal = { throw IllegalStateException("delete failed") },
                afterDelete = { postDeleteCalls++ },
            )
        }

        assertEquals(0, postDeleteCalls)
    }

    @Test
    fun confirmedLocalDeleteRunsPostDeleteActions() {
        var postDeleteCalls = 0

        val deleted = completeLocalNoteDeletion(
            deleteLocal = { true },
            afterDelete = { postDeleteCalls++ },
        )

        assertTrue(deleted)
        assertEquals(1, postDeleteCalls)
    }

    @Test
    fun backupFailureAbortsClearAndPreservesLiveNotes() {
        val root = temporaryFolder.newFolder("clear-files")
        val repository = NotesRepository(root)
        repository.save("a.md", "alpha")
        repository.save("b.md", "beta")
        File(root, NotesRepository.NOTE_BACKUPS_DIR).writeText("blocks backup directory")

        assertThrows(IllegalStateException::class.java) {
            repository.clearAll()
        }

        assertEquals("alpha", repository.read("a.md"))
        assertEquals("beta", repository.read("b.md"))
    }
}

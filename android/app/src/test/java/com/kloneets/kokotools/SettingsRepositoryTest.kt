package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class SettingsRepositoryTest {
    @Test
    fun parsesDesktopCompatibleSubset() {
        val settings = SettingsRepository.parse(
            """
            {
              "pages_app": {"first_book": 100, "second_book": 320, "read_pages": 25},
              "notes_app": {"current_note_path": "books/current.md"},
              "gdrive": {
                "folder_id": "folder-1",
                "folder_name": "Koko",
                "selected_snapshot_id": "snap-1",
                "snapshots": [{"id": "snap-1", "name": "2026", "created_at": "2026-05-13T10:00:00Z"}]
              }
            }
            """.trimIndent(),
        )

        assertEquals(100, settings.pagesApp.firstBook)
        assertEquals(320, settings.pagesApp.secondBook)
        assertEquals(25, settings.pagesApp.readPages)
        assertEquals("books/current.md", settings.notesApp.currentNotePath)
        assertEquals("folder-1", settings.gdrive.folderId)
        assertEquals("snap-1", settings.gdrive.snapshots.single().id)
    }

    @Test
    fun writesDesktopCompatibleFieldNames() {
        val json = SettingsRepository.toJson(
            AppSettings(
                pagesApp = PagesSettings(firstBook = 10, secondBook = 20, readPages = 2),
                notesApp = NotesSettings(currentNotePath = "a.md"),
                gdrive = GDriveSettings(
                    folderId = "folder",
                    folderName = "name",
                    selectedSnapshotId = "snapshot",
                    snapshots = listOf(DriveSnapshotMeta("snapshot", "2026", "now")),
                ),
            ),
        )

        assertEquals(10, json.getJSONObject("pages_app").getInt("first_book"))
        assertEquals(20, json.getJSONObject("pages_app").getInt("second_book"))
        assertEquals(2, json.getJSONObject("pages_app").getInt("read_pages"))
        assertEquals("a.md", json.getJSONObject("notes_app").getString("current_note_path"))
        assertEquals("folder", json.getJSONObject("gdrive").getString("folder_id"))
        assertEquals("snapshot", json.getJSONObject("gdrive").getJSONArray("snapshots").getJSONObject(0).getString("id"))
    }
}

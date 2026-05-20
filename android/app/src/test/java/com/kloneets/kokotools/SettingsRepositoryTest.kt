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
              "notes_app": {"current_note_path": "books/current.md", "preview_hidden": true},
              "android_app": {"theme_mode": "dark"},
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
        assertEquals(true, settings.notesApp.previewHidden)
        assertEquals(ThemeMode.Dark, settings.androidApp.themeMode)
        assertEquals("folder-1", settings.gdrive.folderId)
        assertEquals("snap-1", settings.gdrive.snapshots.single().id)
    }

    @Test
    fun writesDesktopCompatibleFieldNames() {
        val json = SettingsRepository.toJson(
            AppSettings(
                pagesApp = PagesSettings(firstBook = 10, secondBook = 20, readPages = 2),
                notesApp = NotesSettings(currentNotePath = "a.md", previewHidden = true),
                androidApp = AndroidSettings(themeMode = ThemeMode.Light),
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
        assertEquals(true, json.getJSONObject("notes_app").getBoolean("preview_hidden"))
        assertEquals("light", json.getJSONObject("android_app").getString("theme_mode"))
        assertEquals("folder", json.getJSONObject("gdrive").getString("folder_id"))
        assertEquals("snapshot", json.getJSONObject("gdrive").getJSONArray("snapshots").getJSONObject(0).getString("id"))
    }

    @Test
    fun writesDesktopCompatibleDefaultsForMobileSnapshots() {
        val json = SettingsRepository.toJson(
            AppSettings(
                pagesApp = PagesSettings(firstBook = 10),
                notesApp = NotesSettings(currentNotePath = "mobile.md"),
            ),
        )

        assertEquals(true, json.getJSONObject("password_app").getBoolean("letters"))
        assertEquals(true, json.getJSONObject("password_app").getBoolean("numbers"))
        assertEquals(true, json.getJSONObject("password_app").getBoolean("special_symbols"))
        assertEquals(16, json.getJSONObject("password_app").getInt("symbol_count"))
        assertEquals(600, json.getJSONObject("app_window").getInt("width"))
        assertEquals(true, json.getJSONObject("ui").getBoolean("show_notes"))
        assertEquals("tokyo-night", json.getJSONObject("ui").getString("theme"))
        assertEquals("system", json.getJSONObject("android_app").getString("theme_mode"))
        assertEquals(4, json.getJSONObject("notes_app").getInt("tab_spaces"))
        assertEquals(1000, json.getJSONObject("notes_app").getInt("undo_levels"))
        assertEquals("mobile.md", json.getJSONObject("notes_app").getString("current_note_path"))
        assertEquals(10, json.getJSONObject("gdrive").getInt("sync_interval_sec"))
    }

    @Test
    fun preservesDesktopOnlyFieldsWhenWritingAndroidSettings() {
        val loaded = SettingsRepository.parse(
            """
            {
              "password_app": {"letters": true, "numbers": false, "special_symbols": false, "symbol_count": 24},
              "pages_app": {"first_book": 1, "second_book": 2, "read_pages": 3},
              "notes_app": {
                "tab_spaces": 2,
                "undo_levels": 50,
                "editor_width": 42,
                "preview_hidden": true,
                "open_note_paths": ["a.md"],
                "current_note_path": "a.md",
                "sidebar_visible": false,
                "vim_mode": false
              },
              "android_app": {"theme_mode": "dark", "unknown_mobile_field": true},
              "app_window": {"width": 900, "height": 700, "maximized": true},
              "ui": {"show_pages": false, "show_password": true, "show_notes": true, "theme": "gruvbox"},
              "gdrive": {"enabled": true, "sync_interval_sec": 25, "folder_id": "old", "last_sync_status": "ok"}
            }
            """.trimIndent(),
        )

        val json = SettingsRepository.toJson(
            loaded.copy(
                notesApp = loaded.notesApp.copy(currentNotePath = "b.md"),
                gdrive = loaded.gdrive.copy(folderId = "new"),
            ),
        )

        assertEquals(24, json.getJSONObject("password_app").getInt("symbol_count"))
        assertEquals(42, json.getJSONObject("notes_app").getInt("editor_width"))
        assertEquals(true, json.getJSONObject("notes_app").getBoolean("preview_hidden"))
        assertEquals("b.md", json.getJSONObject("notes_app").getString("current_note_path"))
        assertEquals("dark", json.getJSONObject("android_app").getString("theme_mode"))
        assertEquals(true, json.getJSONObject("android_app").getBoolean("unknown_mobile_field"))
        assertEquals(900, json.getJSONObject("app_window").getInt("width"))
        assertEquals("gruvbox", json.getJSONObject("ui").getString("theme"))
        assertEquals(true, json.getJSONObject("gdrive").getBoolean("enabled"))
        assertEquals(25, json.getJSONObject("gdrive").getInt("sync_interval_sec"))
        assertEquals("new", json.getJSONObject("gdrive").getString("folder_id"))
    }

    @Test
    fun invalidAndroidThemeModeFallsBackToSystem() {
        val settings = SettingsRepository.parse("""{"android_app": {"theme_mode": "neon"}}""")

        assertEquals(ThemeMode.System, settings.androidApp.themeMode)
    }
}

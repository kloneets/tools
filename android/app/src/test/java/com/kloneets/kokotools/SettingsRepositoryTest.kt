package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SettingsRepositoryTest {
    @Test
    fun parsesDesktopCompatibleSubset() {
        val settings = SettingsRepository.parse(
            """
            {
              "pages_app": {"first_book": 100, "second_book": 320, "read_pages": 25},
              "notes_app": {
                "current_note_path": "books/current.md",
                "preview_hidden": true,
                "spell_check_enabled": true,
                "spell_dictionaries": ["EN", "lv"]
              },
              "android_app": {"theme_mode": "dark", "last_screen": "sync"},
              "firebase": {"enabled": true, "realtime": true, "workspace_id": "ws-1", "workspace_name": "Team"},
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
        assertEquals(true, settings.notesApp.spellCheckEnabled)
        assertEquals(listOf("EN", "lv"), settings.notesApp.spellDictionaries)
        assertEquals(ThemeMode.Dark, settings.androidApp.themeMode)
        assertEquals("sync", settings.androidApp.lastScreen)
        assertEquals(true, settings.firebase.enabled)
        assertEquals("ws-1", settings.firebase.workspaceId)
    }

    @Test
    fun writesDesktopCompatibleFieldNames() {
        val json = SettingsRepository.toJson(
            AppSettings(
                pagesApp = PagesSettings(firstBook = 10, secondBook = 20, readPages = 2),
                notesApp = NotesSettings(
                    currentNotePath = "a.md",
                    previewHidden = true,
                    spellCheckEnabled = true,
                    spellDictionaries = listOf("en", "lv"),
                ),
                androidApp = AndroidSettings(themeMode = ThemeMode.Light, lastScreen = "pages"),
                firebase = FirebaseSettings(
                    enabled = true,
                    realtime = true,
                    projectId = "project",
                    workspaceId = "ws",
                    workspaceName = "Team",
                ),
            ),
        )

        assertEquals(10, json.getJSONObject("pages_app").getInt("first_book"))
        assertEquals(20, json.getJSONObject("pages_app").getInt("second_book"))
        assertEquals(2, json.getJSONObject("pages_app").getInt("read_pages"))
        assertEquals("a.md", json.getJSONObject("notes_app").getString("current_note_path"))
        assertEquals(true, json.getJSONObject("notes_app").getBoolean("preview_hidden"))
        assertEquals(true, json.getJSONObject("notes_app").getBoolean("spell_check_enabled"))
        assertEquals("en", json.getJSONObject("notes_app").getJSONArray("spell_dictionaries").getString(0))
        assertEquals("lv", json.getJSONObject("notes_app").getJSONArray("spell_dictionaries").getString(1))
        assertEquals("light", json.getJSONObject("android_app").getString("theme_mode"))
        assertEquals("pages", json.getJSONObject("android_app").getString("last_screen"))
        assertEquals(true, json.getJSONObject("firebase").getBoolean("enabled"))
        assertEquals("project", json.getJSONObject("firebase").getString("project_id"))
        assertEquals("ws", json.getJSONObject("firebase").getString("workspace_id"))
        assertFalse(json.has("gdrive"))
    }

    @Test
    fun missingFirebaseConfigUsesBundledDefaults() {
        val settings = SettingsRepository.parse(
            """{"firebase": {"enabled": true, "realtime": true}}""",
            FirebaseBundledDefaults(
                apiKey = "bundled-key",
                databaseUrl = "https://bundled.firebaseio.com",
                projectId = "bundled-project",
            ),
        )

        assertEquals("bundled-key", settings.firebase.apiKey)
        assertEquals("https://bundled.firebaseio.com", settings.firebase.databaseUrl)
        assertEquals("bundled-project", settings.firebase.projectId)
    }

    @Test
    fun customFirebaseConfigIsPreservedOverBundledDefaults() {
        val settings = SettingsRepository.parse(
            """
            {
              "firebase": {
                "api_key": "custom-key",
                "database_url": "https://custom.firebaseio.com",
                "project_id": "custom-project"
              }
            }
            """.trimIndent(),
            FirebaseBundledDefaults(
                apiKey = "bundled-key",
                databaseUrl = "https://bundled.firebaseio.com",
                projectId = "bundled-project",
            ),
        )

        assertEquals("custom-key", settings.firebase.apiKey)
        assertEquals("https://custom.firebaseio.com", settings.firebase.databaseUrl)
        assertEquals("custom-project", settings.firebase.projectId)
    }

    @Test
    fun bundledFirebaseDefaultsArePresentInBuild() {
        assertTrue(FirebaseDefaults.bundled.ready)
        assertEquals("koko-tools", FirebaseDefaults.bundled.projectId)
    }

    @Test
    fun personalFirebaseWorkspaceIdIsStableForUid() {
        assertEquals("user_uid-123", FirebaseSyncRepository.personalWorkspaceId("uid-123"))
    }

    @Test
    fun writesDesktopCompatibleDefaultsWithoutGoogleDrive() {
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
        assertEquals("todo", json.getJSONObject("android_app").getString("last_screen"))
        assertEquals(4, json.getJSONObject("notes_app").getInt("tab_spaces"))
        assertEquals(1000, json.getJSONObject("notes_app").getInt("undo_levels"))
        assertEquals("mobile.md", json.getJSONObject("notes_app").getString("current_note_path"))
        assertEquals(false, json.getJSONObject("notes_app").getBoolean("spell_check_enabled"))
        assertEquals(0, json.getJSONObject("notes_app").getJSONArray("spell_dictionaries").length())
        assertEquals(true, json.getJSONObject("firebase").getBoolean("realtime"))
        assertFalse(json.has("gdrive"))
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
                "spell_check_enabled": true,
                "spell_dictionaries": ["en"],
                "open_note_paths": ["a.md"],
                "current_note_path": "a.md",
                "sidebar_visible": false,
                "vim_mode": false
              },
              "android_app": {"theme_mode": "dark", "last_screen": "notes", "unknown_mobile_field": true},
              "app_window": {"width": 900, "height": 700, "maximized": true},
                "ui": {"show_pages": false, "show_password": true, "show_notes": true, "theme": "gruvbox"},
              "gdrive": {"enabled": true, "sync_interval_sec": 25, "folder_id": "old", "last_sync_status": "ok"}
            }
            """.trimIndent(),
        )

        val json = SettingsRepository.toJson(
            loaded.copy(
                notesApp = loaded.notesApp.copy(currentNotePath = "b.md"),
            ),
        )

        assertEquals(24, json.getJSONObject("password_app").getInt("symbol_count"))
        assertEquals(42, json.getJSONObject("notes_app").getInt("editor_width"))
        assertEquals(true, json.getJSONObject("notes_app").getBoolean("preview_hidden"))
        assertEquals(true, json.getJSONObject("notes_app").getBoolean("spell_check_enabled"))
        assertEquals("en", json.getJSONObject("notes_app").getJSONArray("spell_dictionaries").getString(0))
        assertEquals("b.md", json.getJSONObject("notes_app").getString("current_note_path"))
        assertEquals("dark", json.getJSONObject("android_app").getString("theme_mode"))
        assertEquals("notes", json.getJSONObject("android_app").getString("last_screen"))
        assertEquals(true, json.getJSONObject("android_app").getBoolean("unknown_mobile_field"))
        assertEquals(900, json.getJSONObject("app_window").getInt("width"))
        assertEquals("gruvbox", json.getJSONObject("ui").getString("theme"))
        assertFalse(json.has("gdrive"))
    }

    @Test
    fun invalidAndroidThemeModeFallsBackToSystem() {
        val settings = SettingsRepository.parse("""{"android_app": {"theme_mode": "neon"}}""")

        assertEquals(ThemeMode.System, settings.androidApp.themeMode)
    }

    @Test
    fun invalidAndroidLastScreenFallsBackToTodo() {
        val settings = SettingsRepository.parse("""{"android_app": {"last_screen": "password"}}""")

        assertEquals("todo", settings.androidApp.lastScreen)
    }

    @Test
    fun missingSpellSettingsDefaultToDisabled() {
        val settings = SettingsRepository.parse("""{"notes_app": {"current_note_path": "a.md"}}""")

        assertFalse(settings.notesApp.spellCheckEnabled)
        assertEquals(emptyList<String>(), settings.notesApp.spellDictionaries)
    }

    @Test
    fun sharedSettingsExcludeLocalOnlyState() {
        val settings = SettingsRepository.parse(
            """
            {
              "pages_app": {"first_book": 10, "second_book": 20, "read_pages": 3},
              "password_app": {"letters": false, "numbers": true, "special_symbols": false, "symbol_count": 12},
              "notes_app": {
                "tab_spaces": 2,
                "undo_levels": 50,
                "preview_hidden": true,
                "current_note_path": "private.md",
                "open_note_paths": ["private.md"],
                "vim_mode": false,
                "spell_check_enabled": true,
                "spell_dictionaries": ["en"]
              },
              "android_app": {"theme_mode": "dark"},
              "firebase": {"workspace_id": "user_1"}
            }
            """.trimIndent(),
        )

        val shared = SettingsRepository.sharedSettingsJson(settings)

        assertEquals(10, shared.getJSONObject("pages_app").getInt("first_book"))
        assertEquals(12, shared.getJSONObject("password_app").getInt("symbol_count"))
        assertFalse(shared.getJSONObject("notes_app").has("preview_hidden"))
        assertFalse(shared.getJSONObject("notes_app").has("current_note_path"))
        assertFalse(shared.getJSONObject("notes_app").has("open_note_paths"))
        assertFalse(shared.has("android_app"))
        assertFalse(shared.has("firebase"))
    }

    @Test
    fun applySharedSettingsPreservesLocalOnlyState() {
        val local = SettingsRepository.parse(
            """
            {
              "pages_app": {"first_book": 1, "second_book": 2, "read_pages": 3},
              "notes_app": {"current_note_path": "local.md", "preview_hidden": false},
              "android_app": {"theme_mode": "dark"}
            }
            """.trimIndent(),
        )
        val shared = org.json.JSONObject(
            """
            {
              "pages_app": {"first_book": 99, "second_book": 2, "read_pages": 3},
              "notes_app": {"preview_hidden": true, "spell_check_enabled": true}
            }
            """.trimIndent(),
        )

        val applied = SettingsRepository.applySharedSettings(local, shared)

        assertEquals(99, applied.pagesApp.firstBook)
        assertEquals("local.md", applied.notesApp.currentNotePath)
        assertEquals(false, applied.notesApp.previewHidden)
        assertEquals(true, applied.notesApp.spellCheckEnabled)
        assertEquals(ThemeMode.Dark, applied.androidApp.themeMode)
    }

    @Test
    fun localOnlySettingsDoNotChangeSharedHash() {
        val first = SettingsRepository.parse(
            """
            {
              "pages_app": {"first_book": 10, "second_book": 20, "read_pages": 3},
              "notes_app": {"current_note_path": "a.md", "preview_hidden": true, "spell_check_enabled": true},
              "android_app": {"theme_mode": "dark"},
              "firebase": {"enabled": true, "workspace_id": "user_1"}
            }
            """.trimIndent(),
        )
        val second = SettingsRepository.parse(
            """
            {
              "pages_app": {"first_book": 10, "second_book": 20, "read_pages": 3},
              "notes_app": {"current_note_path": "b.md", "preview_hidden": false, "spell_check_enabled": true},
              "android_app": {"theme_mode": "light"},
              "firebase": {"enabled": false, "workspace_id": "user_2"}
            }
            """.trimIndent(),
        )

        assertEquals(
            FirebaseSyncRepository.sharedSettingsHash(SettingsRepository.sharedSettingsJson(first)),
            FirebaseSyncRepository.sharedSettingsHash(SettingsRepository.sharedSettingsJson(second)),
        )
    }

    @Test
    fun noteInputTypeUsesNativeSpellCheckWhenEnabled() {
        val inputType = NoteEditorInputTypes.forSpellCheck(enabled = true)

        assertTrue(inputType and android.text.InputType.TYPE_TEXT_FLAG_AUTO_CORRECT != 0)
        assertFalse(inputType and android.text.InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS != 0)
    }

    @Test
    fun noteInputTypeDisablesSuggestionsWhenSpellCheckIsOff() {
        val inputType = NoteEditorInputTypes.forSpellCheck(enabled = false)

        assertTrue(inputType and android.text.InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS != 0)
        assertFalse(inputType and android.text.InputType.TYPE_TEXT_FLAG_AUTO_CORRECT != 0)
    }
}

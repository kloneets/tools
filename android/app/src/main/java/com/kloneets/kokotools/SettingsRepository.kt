package com.kloneets.kokotools

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

class SettingsRepository(private val context: Context) {
    private val settingsFile: File
        get() = File(context.filesDir, SETTINGS_FILE)

    fun load(): AppSettings {
        val raw = settingsFile.takeIf { it.isFile }?.readText() ?: return AppSettings()
        return runCatching { parse(raw) }.getOrDefault(AppSettings())
    }

    fun save(settings: AppSettings) {
        context.filesDir.mkdirs()
        settingsFile.writeText(toJson(settings).toString(2))
    }

    fun settingsPath(): File = settingsFile

    companion object {
        const val SETTINGS_FILE = "settings.json"

        fun parse(raw: String): AppSettings {
            val root = JSONObject(raw)
            val pages = root.optJSONObject("pages_app") ?: JSONObject()
            val notes = root.optJSONObject("notes_app") ?: JSONObject()
            val android = root.optJSONObject("android_app") ?: JSONObject()
            val gdrive = root.optJSONObject("gdrive") ?: JSONObject()
            val firebase = root.optJSONObject("firebase") ?: JSONObject()
            val snapshots = gdrive.optJSONArray("snapshots") ?: JSONArray()

            return AppSettings(
                pagesApp = PagesSettings(
                    firstBook = pages.optInt("first_book", 0),
                    secondBook = pages.optInt("second_book", 0),
                    readPages = pages.optInt("read_pages", 0),
                ),
                notesApp = NotesSettings(
                    currentNotePath = notes.optString("current_note_path", ""),
                    previewHidden = notes.optBoolean("preview_hidden", false),
                ),
                androidApp = AndroidSettings(
                    themeMode = ThemeMode.fromValue(android.optString("theme_mode", ThemeMode.System.value)),
                ),
                gdrive = GDriveSettings(
                    folderId = gdrive.optString("folder_id", ""),
                    folderName = gdrive.optString("folder_name", ""),
                    selectedSnapshotId = gdrive.optString("selected_snapshot_id", ""),
                    snapshots = parseSnapshots(snapshots),
                ),
                firebase = FirebaseSettings(
                    enabled = firebase.optBoolean("enabled", false),
                    realtime = firebase.optBoolean("realtime", true),
                    apiKey = firebase.optString("api_key", ""),
                    databaseUrl = firebase.optString("database_url", ""),
                    userEmail = firebase.optString("user_email", ""),
                    workspaceId = firebase.optString("workspace_id", ""),
                    workspaceName = firebase.optString("workspace_name", ""),
                ),
                rawJson = root.toString(),
            )
        }

        fun toJson(settings: AppSettings): JSONObject {
            val root = settings.rawJson
                .takeIf { it.isNotBlank() }
                ?.let { runCatching { JSONObject(it) }.getOrNull() }
                ?: defaultDesktopSettingsJson()

            val pages = root.objectOrPut("pages_app")
            pages.put("first_book", settings.pagesApp.firstBook)
            pages.put("second_book", settings.pagesApp.secondBook)
            pages.put("read_pages", settings.pagesApp.readPages)

            val notes = root.objectOrPut("notes_app")
            notes.put("current_note_path", settings.notesApp.currentNotePath)
            notes.put("preview_hidden", settings.notesApp.previewHidden)
            notes.putIfMissing("tab_spaces", 4)
            notes.putIfMissing("undo_levels", 1000)
            notes.putIfMissing("sidebar_visible", true)
            notes.putIfMissing("vim_mode", true)

            val android = root.objectOrPut("android_app")
            android.put("theme_mode", settings.androidApp.themeMode.value)

            root.objectOrPut("password_app").apply {
                putIfMissing("letters", true)
                putIfMissing("numbers", true)
                putIfMissing("special_symbols", true)
                putIfMissing("symbol_count", 16)
            }

            root.objectOrPut("app_window").apply {
                putIfMissing("width", 600)
                putIfMissing("height", 300)
                putIfMissing("maximized", false)
            }

            root.objectOrPut("ui").apply {
                putIfMissing("show_pages", true)
                putIfMissing("show_password", true)
                putIfMissing("show_notes", true)
                putIfMissing("theme", "tokyo-night")
                putIfMissing("transparent_background", false)
            }

            val gdrive = root.objectOrPut("gdrive")
            gdrive.putIfMissing("enabled", false)
            gdrive.putIfMissing("sync_interval_sec", 10)
            gdrive.put("folder_id", settings.gdrive.folderId)
            gdrive.put("folder_name", settings.gdrive.folderName)
            gdrive.putIfMissing("pending_sync", false)
            gdrive.putIfMissing("last_remote_state", "")
            gdrive.putIfMissing("conflict_remote_state", "")
            gdrive.putIfMissing("last_sync_at", "")
            gdrive.putIfMissing("last_sync_status", "")
            gdrive.putIfMissing("last_sync_message", "")
            gdrive.putIfMissing("last_local_save_at", "")
            gdrive.putIfMissing("last_drive_save_at", "")
            gdrive.putIfMissing("last_drive_refresh_at", "")
            gdrive.put("selected_snapshot_id", settings.gdrive.selectedSnapshotId)
            gdrive.put(
                "snapshots",
                JSONArray(settings.gdrive.snapshots.map { snapshot ->
                    JSONObject()
                        .put("id", snapshot.id)
                        .put("name", snapshot.name)
                        .put("created_at", snapshot.createdAt)
                }),
            )

            val firebase = root.objectOrPut("firebase")
            firebase.put("enabled", settings.firebase.enabled)
            firebase.put("realtime", settings.firebase.realtime)
            firebase.put("api_key", settings.firebase.apiKey)
            firebase.put("database_url", settings.firebase.databaseUrl)
            firebase.put("user_email", settings.firebase.userEmail)
            firebase.put("workspace_id", settings.firebase.workspaceId)
            firebase.put("workspace_name", settings.firebase.workspaceName)
            firebase.putIfMissing("last_sync_at", "")
            firebase.putIfMissing("last_sync_status", "")
            firebase.putIfMissing("last_sync_message", "")

            return root
        }

        private fun parseSnapshots(snapshots: JSONArray): List<DriveSnapshotMeta> {
            return (0 until snapshots.length()).mapNotNull { index ->
                snapshots.optJSONObject(index)?.let {
                    DriveSnapshotMeta(
                        id = it.optString("id", ""),
                        name = it.optString("name", ""),
                        createdAt = it.optString("created_at", ""),
                    )
                }
            }
        }

        private fun defaultDesktopSettingsJson(): JSONObject {
            return JSONObject()
                .put(
                    "password_app",
                    JSONObject()
                        .put("letters", true)
                        .put("numbers", true)
                        .put("special_symbols", true)
                        .put("symbol_count", 16),
                )
                .put(
                    "pages_app",
                    JSONObject()
                        .put("first_book", 0)
                        .put("second_book", 0)
                        .put("read_pages", 0),
                )
                .put(
                    "notes_app",
                    JSONObject()
                        .put("tab_spaces", 4)
                        .put("undo_levels", 1000)
                        .put("current_note_path", "")
                        .put("sidebar_visible", true)
                        .put("vim_mode", true),
                )
                .put(
                    "android_app",
                    JSONObject()
                        .put("theme_mode", ThemeMode.System.value),
                )
                .put(
                    "app_window",
                    JSONObject()
                        .put("width", 600)
                        .put("height", 300)
                        .put("maximized", false),
                )
                .put(
                    "ui",
                    JSONObject()
                        .put("show_pages", true)
                        .put("show_password", true)
                        .put("show_notes", true)
                        .put("theme", "tokyo-night")
                        .put("transparent_background", false),
                )
                .put(
                    "gdrive",
                    JSONObject()
                        .put("enabled", false)
                        .put("sync_interval_sec", 10)
                        .put("folder_id", "")
                        .put("folder_name", "")
                        .put("pending_sync", false)
                        .put("last_remote_state", "")
                        .put("conflict_remote_state", "")
                        .put("last_sync_at", "")
                        .put("last_sync_status", "")
                        .put("last_sync_message", "")
                        .put("last_local_save_at", "")
                        .put("last_drive_save_at", "")
                        .put("last_drive_refresh_at", "")
                        .put("selected_snapshot_id", "")
                        .put("snapshots", JSONArray()),
                )
                .put(
                    "firebase",
                    JSONObject()
                        .put("enabled", false)
                        .put("realtime", true)
                        .put("api_key", "")
                        .put("database_url", "")
                        .put("user_email", "")
                        .put("workspace_id", "")
                        .put("workspace_name", "")
                        .put("last_sync_at", "")
                        .put("last_sync_status", "")
                        .put("last_sync_message", ""),
                )
        }

        private fun JSONObject.objectOrPut(name: String): JSONObject {
            val existing = optJSONObject(name)
            if (existing != null) return existing
            val created = JSONObject()
            put(name, created)
            return created
        }

        private fun JSONObject.putIfMissing(name: String, value: Any) {
            if (!has(name) || isNull(name)) {
                put(name, value)
            }
        }
    }
}

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
            val gdrive = root.optJSONObject("gdrive") ?: JSONObject()
            val snapshots = gdrive.optJSONArray("snapshots") ?: JSONArray()

            return AppSettings(
                pagesApp = PagesSettings(
                    firstBook = pages.optInt("first_book", 0),
                    secondBook = pages.optInt("second_book", 0),
                    readPages = pages.optInt("read_pages", 0),
                ),
                notesApp = NotesSettings(
                    currentNotePath = notes.optString("current_note_path", ""),
                ),
                gdrive = GDriveSettings(
                    folderId = gdrive.optString("folder_id", ""),
                    folderName = gdrive.optString("folder_name", ""),
                    selectedSnapshotId = gdrive.optString("selected_snapshot_id", ""),
                    snapshots = parseSnapshots(snapshots),
                ),
            )
        }

        fun toJson(settings: AppSettings): JSONObject {
            return JSONObject()
                .put(
                    "pages_app",
                    JSONObject()
                        .put("first_book", settings.pagesApp.firstBook)
                        .put("second_book", settings.pagesApp.secondBook)
                        .put("read_pages", settings.pagesApp.readPages),
                )
                .put(
                    "notes_app",
                    JSONObject()
                        .put("current_note_path", settings.notesApp.currentNotePath),
                )
                .put(
                    "gdrive",
                    JSONObject()
                        .put("folder_id", settings.gdrive.folderId)
                        .put("folder_name", settings.gdrive.folderName)
                        .put("selected_snapshot_id", settings.gdrive.selectedSnapshotId)
                        .put(
                            "snapshots",
                            JSONArray(settings.gdrive.snapshots.map { snapshot ->
                                JSONObject()
                                    .put("id", snapshot.id)
                                    .put("name", snapshot.name)
                                    .put("created_at", snapshot.createdAt)
                            }),
                        ),
                )
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
    }
}

package com.kloneets.kokotools

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

class SettingsRepository(private val context: Context) {
    private val settingsFile: File
        get() = File(context.filesDir, SETTINGS_FILE)

    fun load(): AppSettings {
        val raw = settingsFile.takeIf { it.isFile }?.readText() ?: return AppSettings(
            firebase = FirebaseDefaults.applyBundledDefaults(FirebaseSettings()),
        )
        return runCatching { parse(raw) }.getOrElse {
            AppSettings(firebase = FirebaseDefaults.applyBundledDefaults(FirebaseSettings()))
        }
    }

    fun save(settings: AppSettings) {
        context.filesDir.mkdirs()
        settingsFile.writeText(toJson(settings).toString(2))
    }

    fun settingsPath(): File = settingsFile

    companion object {
        const val SETTINGS_FILE = "settings.json"

        fun parse(
            raw: String,
            firebaseDefaults: FirebaseBundledDefaults = FirebaseDefaults.bundled,
        ): AppSettings {
            val root = JSONObject(raw)
            val pages = root.optJSONObject("pages_app") ?: JSONObject()
            val notes = root.optJSONObject("notes_app") ?: JSONObject()
            val android = root.optJSONObject("android_app") ?: JSONObject()
            val firebase = root.optJSONObject("firebase") ?: JSONObject()

            return AppSettings(
                pagesApp = PagesSettings(
                    firstBook = pages.optInt("first_book", 0),
                    secondBook = pages.optInt("second_book", 0),
                    readPages = pages.optInt("read_pages", 0),
                ),
                notesApp = NotesSettings(
                    currentNotePath = notes.optString("current_note_path", ""),
                    previewHidden = notes.optBoolean("preview_hidden", false),
                    spellCheckEnabled = notes.optBoolean("spell_check_enabled", false),
                    spellDictionaries = parseStringArray(notes.optJSONArray("spell_dictionaries")),
                ),
                androidApp = AndroidSettings(
                    themeMode = ThemeMode.fromValue(android.optString("theme_mode", ThemeMode.System.value)),
                    lastScreen = AndroidScreenState.normalize(android.optString("last_screen", AndroidScreenState.TODO)),
                ),
                firebase = FirebaseDefaults.applyBundledDefaults(
                    FirebaseSettings(
                        enabled = firebase.optBoolean("enabled", false),
                        realtime = firebase.optBoolean("realtime", true),
                        apiKey = firebase.optString("api_key", ""),
                        databaseUrl = firebase.optString("database_url", ""),
                        projectId = firebase.optString("project_id", ""),
                        userEmail = firebase.optString("user_email", ""),
                        workspaceId = firebase.optString("workspace_id", ""),
                        workspaceName = firebase.optString("workspace_name", ""),
                        lastSyncAt = firebase.optString("last_sync_at", ""),
                        lastSyncStatus = FirebaseSyncStatus.fromValue(firebase.optString("last_sync_status", "")),
                        lastSyncMessage = firebase.optString("last_sync_message", ""),
                    ),
                    firebaseDefaults,
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
            notes.put("spell_check_enabled", settings.notesApp.spellCheckEnabled)
            notes.put("spell_dictionaries", JSONArray(settings.notesApp.spellDictionaries))
            notes.putIfMissing("tab_spaces", 4)
            notes.putIfMissing("undo_levels", 1000)
            notes.putIfMissing("sidebar_visible", true)
            notes.putIfMissing("vim_mode", true)

            val android = root.objectOrPut("android_app")
            android.put("theme_mode", settings.androidApp.themeMode.value)
            android.put("last_screen", AndroidScreenState.normalize(settings.androidApp.lastScreen))

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
            root.remove("gdrive")

            val firebase = root.objectOrPut("firebase")
            firebase.put("enabled", settings.firebase.enabled)
            firebase.put("realtime", settings.firebase.realtime)
            firebase.put("api_key", settings.firebase.apiKey)
            firebase.put("database_url", settings.firebase.databaseUrl)
            firebase.put("project_id", settings.firebase.projectId)
            firebase.put("user_email", settings.firebase.userEmail)
            firebase.put("workspace_id", settings.firebase.workspaceId)
            firebase.put("workspace_name", settings.firebase.workspaceName)
            firebase.put("last_sync_at", settings.firebase.lastSyncAt)
            firebase.put("last_sync_status", settings.firebase.lastSyncStatus.value)
            firebase.put("last_sync_message", settings.firebase.lastSyncMessage)

            return root
        }

        fun sharedSettingsJson(settings: AppSettings): JSONObject {
            val root = toJson(settings)
            val shared = JSONObject()
            root.optJSONObject("pages_app")?.let { shared.put("pages_app", JSONObject(it.toString())) }
            root.optJSONObject("password_app")?.let { shared.put("password_app", JSONObject(it.toString())) }
            root.optJSONObject("notes_app")?.let { notes ->
                shared.put(
                    "notes_app",
                    JSONObject()
                        .copyIfPresent(notes, "tab_spaces")
                        .copyIfPresent(notes, "undo_levels")
                        .copyIfPresent(notes, "vim_mode")
                        .copyIfPresent(notes, "spell_check_enabled")
                        .copyIfPresent(notes, "spell_dictionaries"),
                )
            }
            return shared
        }

        fun applySharedSettings(settings: AppSettings, shared: JSONObject): AppSettings {
            val root = toJson(settings)
            shared.optJSONObject("pages_app")?.let { root.put("pages_app", JSONObject(it.toString())) }
            shared.optJSONObject("password_app")?.let { root.put("password_app", JSONObject(it.toString())) }
            shared.optJSONObject("notes_app")?.let { remote ->
                val notes = root.objectOrPut("notes_app")
                listOf(
                    "tab_spaces",
                    "undo_levels",
                    "vim_mode",
                    "spell_check_enabled",
                    "spell_dictionaries",
                ).forEach { key ->
                    if (remote.has(key)) notes.put(key, remote.get(key))
                }
            }
            return parse(root.toString())
        }

        private fun parseStringArray(values: JSONArray?): List<String> {
            if (values == null) return emptyList()
            return (0 until values.length()).mapNotNull { index ->
                values.optString(index, "").trim().takeIf { it.isNotEmpty() }
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
                        .put("spell_check_enabled", false)
                        .put("spell_dictionaries", JSONArray())
                        .put("sidebar_visible", true)
                        .put("vim_mode", true),
                )
                .put(
                    "android_app",
                    JSONObject()
                        .put("theme_mode", ThemeMode.System.value)
                        .put("last_screen", AndroidScreenState.TODO),
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
                    "firebase",
                    JSONObject()
                        .put("enabled", false)
                        .put("realtime", true)
                        .put("api_key", "")
                        .put("database_url", "")
                        .put("project_id", "")
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

        private fun JSONObject.copyIfPresent(source: JSONObject, name: String): JSONObject {
            if (source.has(name) && !source.isNull(name)) {
                put(name, source.get(name))
            }
            return this
        }
    }
}

data class FirebaseBundledDefaults(
    val apiKey: String = "",
    val databaseUrl: String = "",
    val projectId: String = "",
) {
    val ready: Boolean
        get() = apiKey.isNotBlank() && databaseUrl.isNotBlank()
}

object FirebaseDefaults {
    val bundled: FirebaseBundledDefaults
        get() = FirebaseBundledDefaults(
            apiKey = BuildConfig.FIREBASE_API_KEY,
            databaseUrl = BuildConfig.FIREBASE_DATABASE_URL,
            projectId = BuildConfig.FIREBASE_PROJECT_ID,
        )

    fun applyBundledDefaults(
        settings: FirebaseSettings,
        defaults: FirebaseBundledDefaults = bundled,
    ): FirebaseSettings {
        return settings.copy(
            apiKey = settings.apiKey.ifBlank { defaults.apiKey },
            databaseUrl = settings.databaseUrl.ifBlank { defaults.databaseUrl },
            projectId = settings.projectId.ifBlank { defaults.projectId },
        )
    }
}

object AndroidScreenState {
    const val TODO = "todo"
    const val NOTES = "notes"
    const val PAGES = "pages"
    const val SYNC = "sync"
    const val SETTINGS = "settings"

    private val valid = setOf(TODO, NOTES, PAGES, SYNC, SETTINGS)

    fun normalize(value: String): String {
        return value.trim().lowercase().takeIf { it in valid } ?: TODO
    }
}

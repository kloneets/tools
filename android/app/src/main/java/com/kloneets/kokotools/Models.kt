package com.kloneets.kokotools

data class AppSettings(
    val pagesApp: PagesSettings = PagesSettings(),
    val notesApp: NotesSettings = NotesSettings(),
    val androidApp: AndroidSettings = AndroidSettings(),
    val firebase: FirebaseSettings = FirebaseSettings(),
    val rawJson: String = "",
)

data class PagesSettings(
    val firstBook: Int = 0,
    val secondBook: Int = 0,
    val readPages: Int = 0,
)

data class NotesSettings(
    val currentNotePath: String = "",
    val previewHidden: Boolean = false,
    val spellCheckEnabled: Boolean = false,
    val spellDictionaries: List<String> = emptyList(),
)

data class AndroidSettings(
    val themeMode: ThemeMode = ThemeMode.System,
    val lastScreen: String = "todo",
)

enum class ThemeMode(val value: String, val label: String) {
    Light("light", "Light"),
    Dark("dark", "Dark"),
    System("system", "System");

    companion object {
        fun fromValue(value: String): ThemeMode {
            return values().firstOrNull { it.value == value } ?: System
        }
    }
}

data class FirebaseSettings(
    val enabled: Boolean = false,
    val realtime: Boolean = true,
    val apiKey: String = "",
    val databaseUrl: String = "",
    val projectId: String = "",
    val userEmail: String = "",
    val workspaceId: String = "",
    val workspaceName: String = "",
    val lastSyncAt: String = "",
    val lastSyncStatus: FirebaseSyncStatus = FirebaseSyncStatus.None,
    val lastSyncMessage: String = "",
)

enum class FirebaseSyncStatus(val value: String) {
    None(""),
    Success("ok"),
    Error("error");

    companion object {
        fun fromValue(value: String): FirebaseSyncStatus {
            return values().firstOrNull { it.value == value.trim().lowercase() } ?: None
        }
    }
}

data class NoteFile(
    val relativePath: String,
    val displayName: String = relativePath,
)

data class PagesResult(
    val readPages: Int,
    val firstBookPages: Int,
    val secondBookPages: Int,
    val convertedPages: Int,
    val percent: Int,
) {
    val label: String
        get() = "$convertedPages pages, $percent%"
}

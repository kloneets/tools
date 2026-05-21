package com.kloneets.kokotools

data class AppSettings(
    val pagesApp: PagesSettings = PagesSettings(),
    val notesApp: NotesSettings = NotesSettings(),
    val androidApp: AndroidSettings = AndroidSettings(),
    val gdrive: GDriveSettings = GDriveSettings(),
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
)

data class AndroidSettings(
    val themeMode: ThemeMode = ThemeMode.System,
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

data class GDriveSettings(
    val folderId: String = "",
    val folderName: String = "",
    val selectedSnapshotId: String = "",
    val snapshots: List<DriveSnapshotMeta> = emptyList(),
)

data class DriveSnapshotMeta(
    val id: String,
    val name: String,
    val createdAt: String,
)

data class FirebaseSettings(
    val enabled: Boolean = false,
    val realtime: Boolean = true,
    val apiKey: String = "",
    val databaseUrl: String = "",
    val userEmail: String = "",
    val workspaceId: String = "",
    val workspaceName: String = "",
)

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

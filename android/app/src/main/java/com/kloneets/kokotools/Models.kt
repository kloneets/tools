package com.kloneets.kokotools

data class AppSettings(
    val pagesApp: PagesSettings = PagesSettings(),
    val notesApp: NotesSettings = NotesSettings(),
    val gdrive: GDriveSettings = GDriveSettings(),
)

data class PagesSettings(
    val firstBook: Int = 0,
    val secondBook: Int = 0,
    val readPages: Int = 0,
)

data class NotesSettings(
    val currentNotePath: String = "",
)

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

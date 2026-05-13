package com.kloneets.kokotools

object DriveSnapshotSelection {
    fun preserveIfPresent(selectedSnapshotId: String, snapshots: List<DriveSnapshotMeta>): String {
        if (selectedSnapshotId.isBlank()) return ""
        return selectedSnapshotId.takeIf { selected ->
            snapshots.any { it.id == selected }
        }.orEmpty()
    }
}

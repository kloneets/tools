package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class DriveSnapshotSelectionTest {
    @Test
    fun preservesSelectedSnapshotWhenStillPresent() {
        val snapshots = listOf(
            DriveSnapshotMeta("older", "older", "2026-01-01T00:00:00Z"),
            DriveSnapshotMeta("selected", "selected", "2026-05-13T00:00:00Z"),
        )

        assertEquals("selected", DriveSnapshotSelection.preserveIfPresent("selected", snapshots))
    }

    @Test
    fun clearsSelectedSnapshotWhenMissing() {
        val snapshots = listOf(
            DriveSnapshotMeta("remote", "remote", "2026-05-13T00:00:00Z"),
        )

        assertEquals("", DriveSnapshotSelection.preserveIfPresent("local-only", snapshots))
    }
}

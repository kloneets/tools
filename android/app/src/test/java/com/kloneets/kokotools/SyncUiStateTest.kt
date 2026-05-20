package com.kloneets.kokotools

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SyncUiStateTest {
    @Test
    fun disconnectedShowsOnlyConnect() {
        val visibility = SyncUiState.actionVisibility(
            connected = false,
            authInProgress = false,
            hasFolder = false,
            hasSelectedSnapshot = false,
        )

        assertTrue(visibility.connect)
        assertFalse(visibility.folderSelection)
        assertFalse(visibility.upload)
        assertFalse(visibility.refresh)
    }

    @Test
    fun connectedWithoutFolderShowsFolderSelectionOnly() {
        val visibility = SyncUiState.actionVisibility(
            connected = true,
            authInProgress = false,
            hasFolder = false,
            hasSelectedSnapshot = false,
        )

        assertFalse(visibility.connect)
        assertTrue(visibility.folderSelection)
        assertFalse(visibility.upload)
        assertFalse(visibility.refresh)
    }

    @Test
    fun syncedFolderShowsSnapshotActions() {
        val visibility = SyncUiState.actionVisibility(
            connected = true,
            authInProgress = false,
            hasFolder = true,
            hasSelectedSnapshot = false,
        )

        assertTrue(visibility.folderSelection)
        assertTrue(visibility.upload)
        assertTrue(visibility.refresh)
    }
}

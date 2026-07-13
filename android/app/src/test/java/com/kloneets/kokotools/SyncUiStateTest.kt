package com.kloneets.kokotools

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SyncUiStateTest {
    @Test
    fun firebaseProblemIsHiddenWhenDisabled() {
        assertFalse(
            SyncUiState.hasFirebaseProblem(
                FirebaseSettings(enabled = false, lastSyncStatus = FirebaseSyncStatus.Error),
                backendConfigured = false,
                hasSavedSession = false,
            ),
        )
    }

    @Test
    fun enabledFirebaseRequiresConfigWorkspaceAndAuthentication() {
        val configured = FirebaseSettings(enabled = true, workspaceId = "user_1")
        assertTrue(SyncUiState.hasFirebaseProblem(configured, false, true))
        assertTrue(SyncUiState.hasFirebaseProblem(configured.copy(workspaceId = ""), true, true))
        assertTrue(SyncUiState.hasFirebaseProblem(configured, true, false))
        assertFalse(SyncUiState.hasFirebaseProblem(configured, true, true))
    }

    @Test
    fun latestErrorRemainsAProblemUntilSuccess() {
        val firebase = FirebaseSettings(enabled = true, workspaceId = "user_1")
        assertTrue(
            SyncUiState.hasFirebaseProblem(
                firebase.copy(lastSyncStatus = FirebaseSyncStatus.Error),
                backendConfigured = true,
                hasSavedSession = true,
            ),
        )
        assertFalse(
            SyncUiState.hasFirebaseProblem(
                firebase.copy(lastSyncStatus = FirebaseSyncStatus.Success),
                backendConfigured = true,
                hasSavedSession = true,
            ),
        )
    }

    @Test
    fun unchangedFirebaseTodosDoNotRebuildVisibleTodoScreen() {
        assertFalse(
            SyncUiState.shouldRebuildTodoAfterPull(
                todoChanged = false,
                showingTodo = true,
                canRebuild = true,
            ),
        )
    }

    @Test
    fun changedFirebaseTodosRebuildOnlyWhenTodoScreenCanRebuild() {
        assertTrue(
            SyncUiState.shouldRebuildTodoAfterPull(
                todoChanged = true,
                showingTodo = true,
                canRebuild = true,
            ),
        )
        assertFalse(
            SyncUiState.shouldRebuildTodoAfterPull(
                todoChanged = true,
                showingTodo = false,
                canRebuild = true,
            ),
        )
        assertFalse(
            SyncUiState.shouldRebuildTodoAfterPull(
                todoChanged = true,
                showingTodo = true,
                canRebuild = false,
            ),
        )
    }
}

package com.kloneets.kokotools

object SyncUiState {
    fun hasFirebaseProblem(
        firebase: FirebaseSettings,
        backendConfigured: Boolean,
        hasSavedSession: Boolean,
    ): Boolean {
        if (!firebase.enabled) return false
        return !backendConfigured ||
            firebase.workspaceId.isBlank() ||
            !hasSavedSession ||
            firebase.lastSyncStatus == FirebaseSyncStatus.Error
    }

    fun shouldRebuildTodoAfterPull(
        todoChanged: Boolean,
        showingTodo: Boolean,
        canRebuild: Boolean,
    ): Boolean {
        return todoChanged && showingTodo && canRebuild
    }
}

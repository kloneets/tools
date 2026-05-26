package com.kloneets.kokotools

data class SyncActionVisibility(
    val connect: Boolean,
    val folderSelection: Boolean,
    val upload: Boolean,
    val refresh: Boolean,
    val realtimeLogin: Boolean = false,
    val workspaceSelection: Boolean = false,
    val memberManagement: Boolean = false,
)

enum class SyncMode {
    FirebaseRealtime,
    LegacyDriveBackup,
}

object SyncUiState {
    fun shouldRebuildTodoAfterPull(
        todoChanged: Boolean,
        showingTodo: Boolean,
        canRebuild: Boolean,
    ): Boolean {
        return todoChanged && showingTodo && canRebuild
    }

    fun actionVisibility(
        connected: Boolean,
        authInProgress: Boolean,
        hasFolder: Boolean,
        hasSelectedSnapshot: Boolean,
        mode: SyncMode = SyncMode.LegacyDriveBackup,
        firebaseLoggedIn: Boolean = false,
        hasWorkspace: Boolean = false,
    ): SyncActionVisibility {
        if (mode == SyncMode.FirebaseRealtime) {
            return SyncActionVisibility(
                connect = false,
                folderSelection = false,
                upload = false,
                refresh = false,
                realtimeLogin = !authInProgress && !firebaseLoggedIn,
                workspaceSelection = !authInProgress && firebaseLoggedIn,
                memberManagement = !authInProgress && firebaseLoggedIn && hasWorkspace,
            )
        }
        if (!connected) {
            return SyncActionVisibility(
                connect = true,
                folderSelection = false,
                upload = false,
                refresh = false,
            )
        }
        return SyncActionVisibility(
            connect = false,
            folderSelection = !authInProgress,
            upload = !authInProgress && hasFolder,
            refresh = !authInProgress && hasFolder,
        )
    }
}

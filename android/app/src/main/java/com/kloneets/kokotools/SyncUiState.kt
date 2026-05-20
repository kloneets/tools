package com.kloneets.kokotools

data class SyncActionVisibility(
    val connect: Boolean,
    val folderSelection: Boolean,
    val upload: Boolean,
    val refresh: Boolean,
)

object SyncUiState {
    fun actionVisibility(
        connected: Boolean,
        authInProgress: Boolean,
        hasFolder: Boolean,
        hasSelectedSnapshot: Boolean,
    ): SyncActionVisibility {
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

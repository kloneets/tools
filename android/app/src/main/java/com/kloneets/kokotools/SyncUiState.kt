package com.kloneets.kokotools

object SyncUiState {
    fun shouldRebuildTodoAfterPull(
        todoChanged: Boolean,
        showingTodo: Boolean,
        canRebuild: Boolean,
    ): Boolean {
        return todoChanged && showingTodo && canRebuild
    }
}

package com.kloneets.kokotools

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SyncUiStateTest {
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

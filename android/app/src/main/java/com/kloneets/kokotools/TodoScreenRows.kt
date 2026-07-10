package com.kloneets.kokotools

data class TodoDisplaySection(
    val title: String,
    val emptyText: String,
    val items: List<TodoItem>,
)

object TodoScreenRows {
    fun activeSections(store: TodoStore): List<TodoDisplaySection> {
        return listOf(
            TodoDisplaySection(
                title = "Short term",
                emptyText = "No short term todos",
                items = TodoRepository.shortItems(store),
            ),
            TodoDisplaySection(
                title = "Long term",
                emptyText = "No long term todos",
                items = TodoRepository.longItems(store),
            ),
        )
    }
}

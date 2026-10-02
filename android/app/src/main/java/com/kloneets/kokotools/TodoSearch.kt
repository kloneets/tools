package com.kloneets.kokotools

/** Positions are UTF-16 offsets, suitable for Android text spans. */
fun fuzzyTodoMatch(text: String, query: String): List<Int>? {
    if (query.isEmpty()) return emptyList()
    val wanted = query.codePoints().toArray()
    val positions = mutableListOf<Int>()
    var index = 0
    var offset = 0
    while (offset < text.length && index < wanted.size) {
        val character = text.codePointAt(offset)
        if (Character.toLowerCase(Character.toUpperCase(character)) ==
            Character.toLowerCase(Character.toUpperCase(wanted[index]))) {
            positions.add(offset)
            index++
        }
        offset += Character.charCount(character)
    }
    return positions.takeIf { index == wanted.size }
}

class TodoSearch {
    var open = false
    var query = ""
    var selectedId: String? = null
        private set
    var results: List<String> = emptyList()
        private set
    private var listId: String? = null

    fun selectList(id: String) {
        if (listId != id) {
            close()
            listId = id
        }
    }

    fun close() {
        open = false
        query = ""
        selectedId = null
        results = emptyList()
    }

    fun refresh(store: TodoStore) {
        val ordered = TodoScreenRows.activeSections(store).flatMap { it.items } +
            TodoRepository.archiveGroups(store).toSortedMap(reverseOrder()).values.flatten()
        results = if (query.isEmpty()) emptyList() else ordered.filter {
            fuzzyTodoMatch(it.text, query) != null
        }.map { it.id }.distinct()
        if (selectedId !in results) selectedId = results.firstOrNull()
    }

    fun move(direction: Int) {
        if (results.isEmpty()) return
        val index = results.indexOf(selectedId).coerceAtLeast(0)
        selectedId = results[Math.floorMod(index + direction, results.size)]
    }

    fun counter(): String = when {
        query.isEmpty() -> "0 / 0"
        results.isEmpty() -> "No matches"
        else -> "${results.indexOf(selectedId) + 1} / ${results.size}"
    }
}

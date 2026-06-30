package com.kloneets.kokotools

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter

data class TodoStore(
    val version: Int = TodoRepository.SCHEMA_VERSION,
    val items: List<TodoItem> = emptyList(),
    val archiveMonths: List<String> = emptyList(),
)

data class TodoItem(
    val id: String,
    val text: String,
    val status: String,
    val order: Int,
    val createdAt: OffsetDateTime,
    val updatedAt: OffsetDateTime,
    val checkedAt: OffsetDateTime? = null,
    val doneAt: OffsetDateTime? = null,
    val archivedAt: OffsetDateTime? = null,
)

class TodoRepository(private val context: Context) {
    fun todosPath(): File = File(context.filesDir, TODOS_FILE)

    fun load(now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoStore {
        val file = todosPath()
        if (!file.exists()) return TodoStore()
        val raw = file.readText()
        if (raw.isBlank()) return TodoStore()
        val json = JSONObject(raw)
        val items = json.optJSONArray("items") ?: JSONArray()
        val archiveMonths = json.optJSONArray("archive_months") ?: JSONArray()
        val store = TodoStore(
            version = SCHEMA_VERSION,
            items = (0 until items.length()).mapNotNull { index ->
                items.optJSONObject(index)?.let { parseItem(it) }
            },
            archiveMonths = (0 until archiveMonths.length()).mapNotNull { index ->
                archiveMonths.optString(index).takeIf { it.isNotBlank() }
            },
        )
        val cleaned = cleanup(normalize(store), now)
        if (cleaned != store) save(cleaned)
        return cleaned
    }

    fun save(store: TodoStore) {
        val normalized = normalize(store)
        val json = JSONObject()
            .put("version", SCHEMA_VERSION)
            .put("items", JSONArray().apply {
                normalized.items.forEach { put(itemJson(it)) }
            })
            .put("archive_months", JSONArray().apply {
                normalized.archiveMonths.forEach { put(it) }
            })
        todosPath().writeText(json.toString(2))
    }

    fun add(text: String): TodoStore {
        val trimmed = text.trim()
        if (trimmed.isBlank()) return load()
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val item = TodoItem(
            id = now.toInstant().toEpochMilli().toString() + "-" + store.items.size,
            text = trimmed,
            status = STATUS_TODO,
            order = nextActiveOrder(store.items),
            createdAt = now,
            updatedAt = now,
        )
        return store.copy(items = store.items + item).also { save(it) }
    }

    fun toggle(id: String): TodoStore {
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val nextOrder = nextActiveOrder(store.items)
        val updated = store.items.map { item ->
            if (item.id != id) return@map item
            if (item.status == STATUS_TODO && item.checkedAt == null) {
                item.copy(checkedAt = now, updatedAt = now)
            } else {
                item.copy(
                    status = STATUS_TODO,
                    order = nextOrder,
                    checkedAt = null,
                    doneAt = null,
                    archivedAt = null,
                    updatedAt = now,
                )
            }
        }
        return store.copy(items = updated).also { save(it) }
    }

    fun edit(id: String, text: String): TodoStore {
        val trimmed = text.trim()
        if (trimmed.isBlank()) return load()
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val updated = store.items.map { item ->
            if (item.id == id && item.status == STATUS_TODO && item.checkedAt == null) {
                item.copy(text = trimmed, updatedAt = now)
            } else {
                item
            }
        }
        return store.copy(items = updated).also { save(it) }
    }

    fun move(id: String, delta: Int): TodoStore {
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val active = activeItems(store).filter { it.checkedAt == null }
        val index = active.indexOfFirst { it.id == id }
        val target = index + delta
        if (index < 0 || target < 0 || target >= active.size) return store
        val a = active[index]
        val b = active[target]
        val updated = store.items.map {
            when (it.id) {
                a.id -> it.copy(order = b.order, updatedAt = now)
                b.id -> it.copy(order = a.order, updatedAt = now)
                else -> it
            }
        }
        return store.copy(items = updated).also { save(it) }
    }

    fun moveTo(draggedId: String, targetId: String): TodoStore {
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val updated = reorderActiveUnchecked(store, draggedId, targetId, now)
        if (updated == store) return store
        save(updated)
        return updated
    }

    companion object {
        const val TODOS_FILE = "todos.json"
        const val SCHEMA_VERSION = 1
        const val STATUS_TODO = "todo"
        const val STATUS_DONE = "done"
        const val STATUS_ARCHIVED = "archived"
        const val CHECKED_DELAY_SECONDS = 10L
        const val ARCHIVE_AFTER_DAYS = 7L

        fun activeItems(store: TodoStore): List<TodoItem> {
            return store.items
                .filter { it.status == STATUS_TODO }
                .sortedWith(compareBy<TodoItem> { it.checkedAt != null }.thenBy { it.order }.thenBy { it.createdAt })
        }

        fun doneItems(store: TodoStore): List<TodoItem> {
            return store.items
                .filter { it.status == STATUS_DONE }
                .sortedByDescending { it.doneAt ?: it.updatedAt }
        }

        fun archiveGroups(store: TodoStore): Map<String, List<TodoItem>> {
            return store.items
                .filter { it.status == STATUS_ARCHIVED && it.archivedAt != null }
                .groupBy { it.archivedAt!!.format(DateTimeFormatter.ofPattern("yyyy-MM")) }
                .toSortedMap(compareByDescending { it })
                .mapValues { (_, items) -> items.sortedByDescending { it.archivedAt } }
        }

        fun archiveMonths(store: TodoStore): List<String> {
            return normalizeArchiveMonths(store.archiveMonths + archiveGroups(store).keys)
        }

        fun archiveMonthItems(store: TodoStore, month: String): List<TodoItem> {
            return archiveGroups(store)[month].orEmpty()
        }

        fun nonArchivedStore(store: TodoStore): TodoStore {
            return store.copy(items = store.items.filter { it.status != STATUS_ARCHIVED })
        }

        fun mergeArchiveMonth(store: TodoStore, month: String, items: List<TodoItem>): TodoStore {
            val storeWithMonth = store.copy(archiveMonths = normalizeArchiveMonths(store.archiveMonths + month))
            val monthItems = items.filter {
                it.status == STATUS_ARCHIVED &&
                    it.archivedAt != null &&
                    it.archivedAt.format(DateTimeFormatter.ofPattern("yyyy-MM")) == month
            }
            if (monthItems.isEmpty()) return storeWithMonth
            val remaining = storeWithMonth.items.filterNot {
                it.status == STATUS_ARCHIVED &&
                    it.archivedAt != null &&
                    it.archivedAt.format(DateTimeFormatter.ofPattern("yyyy-MM")) == month
            }
            return normalize(storeWithMonth.copy(items = remaining + monthItems))
        }

        fun preserveArchived(local: TodoStore, synced: TodoStore): TodoStore {
            val syncedIds = synced.items
                .filter { it.status != STATUS_ARCHIVED }
                .map { it.id }
                .toSet()
            return normalize(synced.copy(
                items = synced.items.filter { it.status != STATUS_ARCHIVED } +
                    local.items.filter { it.status == STATUS_ARCHIVED && !syncedIds.contains(it.id) },
                archiveMonths = local.archiveMonths,
            ))
        }

        fun normalize(store: TodoStore): TodoStore {
            return store.copy(archiveMonths = normalizeArchiveMonths(store.archiveMonths + archiveGroups(store).keys))
        }

        private fun normalizeArchiveMonths(months: Iterable<String>): List<String> {
            return months
                .map { it.trim() }
                .filter { it.length == "yyyy-MM".length }
                .distinct()
                .sortedDescending()
        }

        fun cleanup(store: TodoStore, now: OffsetDateTime): TodoStore {
            var changed = false
            val updated = store.items.map { item ->
                var next = item
                if (next.status == STATUS_TODO && next.checkedAt != null && !now.isBefore(next.checkedAt.plusSeconds(CHECKED_DELAY_SECONDS))) {
                    next = next.copy(
                        status = STATUS_DONE,
                        doneAt = next.doneAt ?: next.checkedAt.plusSeconds(CHECKED_DELAY_SECONDS),
                        updatedAt = now,
                    )
                    changed = true
                }
                if (next.status == STATUS_DONE && next.doneAt != null && !now.isBefore(next.doneAt.plusDays(ARCHIVE_AFTER_DAYS))) {
                    next = next.copy(status = STATUS_ARCHIVED, archivedAt = now, updatedAt = now)
                    changed = true
                }
                next
            }
            return if (changed) normalize(store.copy(items = updated)) else normalize(store)
        }

        fun reorderActiveUnchecked(
            store: TodoStore,
            draggedId: String,
            targetId: String,
            now: OffsetDateTime,
        ): TodoStore {
            if (draggedId == targetId) return store
            val active = activeItems(store).filter { it.checkedAt == null }
            val from = active.indexOfFirst { it.id == draggedId }
            val to = active.indexOfFirst { it.id == targetId }
            if (from < 0 || to < 0) return store

            val reordered = active.toMutableList()
            val dragged = reordered.removeAt(from)
            reordered.add(to, dragged)
            val orderById = reordered.mapIndexed { index, item -> item.id to index }.toMap()
            val updated = store.items.map { item ->
                val order = orderById[item.id] ?: return@map item
                item.copy(order = order, updatedAt = now)
            }
            return store.copy(items = updated)
        }

        private fun nextActiveOrder(items: List<TodoItem>): Int {
            return items.filter { it.status == STATUS_TODO && it.checkedAt == null }
                .maxOfOrNull { it.order + 1 } ?: 0
        }

        fun parseItem(json: JSONObject): TodoItem {
            val created = parseTime(json.optString("created_at")) ?: OffsetDateTime.now(ZoneOffset.UTC)
            return TodoItem(
                id = json.optString("id"),
                text = json.optString("text"),
                status = json.optString("status", STATUS_TODO),
                order = json.optInt("order"),
                createdAt = created,
                updatedAt = parseTime(json.optString("updated_at")) ?: created,
                checkedAt = parseTime(json.optString("checked_at")),
                doneAt = parseTime(json.optString("done_at")),
                archivedAt = parseTime(json.optString("archived_at")),
            )
        }

        fun itemJson(item: TodoItem): JSONObject {
            return JSONObject()
                .put("id", item.id)
                .put("text", item.text)
                .put("status", item.status)
                .put("order", item.order)
                .put("created_at", formatTime(item.createdAt))
                .put("updated_at", formatTime(item.updatedAt))
                .put("checked_at", item.checkedAt?.let { formatTime(it) })
                .put("done_at", item.doneAt?.let { formatTime(it) })
                .put("archived_at", item.archivedAt?.let { formatTime(it) })
        }

        fun parseTime(value: String): OffsetDateTime? {
            if (value.isBlank() || value == "null") return null
            return runCatching { OffsetDateTime.parse(value) }.getOrNull()
        }

        fun formatTime(value: OffsetDateTime): String {
            return value.withOffsetSameInstant(ZoneOffset.UTC).format(DateTimeFormatter.ISO_OFFSET_DATE_TIME)
        }
    }
}

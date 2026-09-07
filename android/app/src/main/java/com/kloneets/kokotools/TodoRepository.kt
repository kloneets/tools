package com.kloneets.kokotools

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.UUID

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
    val term: String = TodoRepository.TERM_SHORT,
)

data class TodoListMeta(
    val id: String,
    val name: String,
    val rev: Long,
    val createdAt: OffsetDateTime,
    val updatedAt: OffsetDateTime,
    val deleted: Boolean = false,
    val deletedAt: OffsetDateTime? = null,
)

data class TodoListsStore(
    val version: Int = TodoRepository.SCHEMA_VERSION,
    val currentListId: String = TodoRepository.DEFAULT_LIST_ID,
    val lists: List<TodoListMeta> = emptyList(),
)

class TodoRepository(
    private val context: Context,
    private val idGenerator: () -> String = { UUID.randomUUID().toString() },
) {
    private var currentListId: String = DEFAULT_LIST_ID

    fun todosPath(): File = File(context.filesDir, TODOS_FILE)

    fun listsPath(): File = File(context.filesDir, LISTS_FILE)

    fun currentListId(): String = normalizeCurrentListId(currentListId)

    fun load(now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoStore {
        return loadList(currentListId(), now)
    }

    fun loadList(id: String, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoStore {
        val file = storePath(id)
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
        if (cleaned != store) saveList(id, cleaned)
        return cleaned
    }

    fun save(store: TodoStore) {
        saveList(currentListId(), store)
    }

    fun saveList(id: String, store: TodoStore) {
        val normalized = normalize(store)
        val json = JSONObject()
            .put("version", SCHEMA_VERSION)
            .put("items", JSONArray().apply {
                normalized.items.forEach { put(itemJson(it)) }
            })
            .put("archive_months", JSONArray().apply {
                normalized.archiveMonths.forEach { put(it) }
            })
        val file = storePath(id)
        file.parentFile?.mkdirs()
        file.writeText(json.toString(2))
    }

    fun loadLists(now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoListsStore {
        val file = listsPath()
        val catalog = if (!file.exists() || file.readText().isBlank()) {
            normalizeLists(TodoListsStore(), now)
        } else {
            val json = JSONObject(file.readText())
            val values = json.optJSONArray("lists") ?: JSONArray()
            normalizeLists(
                TodoListsStore(
                    version = SCHEMA_VERSION,
                    currentListId = json.optString("current_list_id", DEFAULT_LIST_ID),
                    lists = (0 until values.length()).mapNotNull { index ->
                        values.optJSONObject(index)?.let { parseListMeta(it) }
                    },
                ),
                now,
            )
        }
        currentListId = if (activeLists(catalog).any { it.id == currentListId() }) currentListId() else DEFAULT_LIST_ID
        return catalog.copy(currentListId = currentListId())
    }

    fun saveLists(catalog: TodoListsStore, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)) {
        val normalized = normalizeLists(catalog, now)
        currentListId = if (activeLists(normalized).any { it.id == currentListId() }) currentListId() else DEFAULT_LIST_ID
        val json = JSONObject()
            .put("version", SCHEMA_VERSION)
            .put("current_list_id", currentListId())
            .put("lists", JSONArray().apply {
                normalized.lists.forEach { put(listMetaJson(it)) }
            })
        val file = listsPath()
        file.parentFile?.mkdirs()
        file.writeText(json.toString(2))
    }

    fun selectList(id: String, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoListsStore {
        val normalizedId = normalizeListId(id).ifBlank { DEFAULT_LIST_ID }
        val catalog = loadLists(now)
        if (activeLists(catalog).none { it.id == normalizedId }) {
            currentListId = DEFAULT_LIST_ID
            return catalog.copy(currentListId = DEFAULT_LIST_ID)
        }
        currentListId = normalizedId
        return catalog.copy(currentListId = normalizedId)
    }

    fun createList(name: String, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): Pair<TodoListsStore, TodoListMeta> {
        val normalizedName = normalizeListName(name)
        require(normalizedName.isNotBlank()) { "Todo list name is required" }
        val catalog = loadLists(now)
        require(!listNameTaken(catalog, normalizedName, "")) { "Todo list \"$normalizedName\" already exists" }
        val id = generateUniqueListId(catalog)
        val meta = TodoListMeta(
            id = id,
            name = normalizedName,
            rev = revForTime(now),
            createdAt = now.withOffsetSameInstant(ZoneOffset.UTC),
            updatedAt = now.withOffsetSameInstant(ZoneOffset.UTC),
        )
        currentListId = id
        val next = catalog.copy(currentListId = id, lists = catalog.lists + meta)
        saveLists(next, now)
        saveList(id, TodoStore())
        return loadLists(now) to meta
    }

    fun renameList(id: String, name: String, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoListsStore {
        val normalizedId = normalizeListId(id).ifBlank { DEFAULT_LIST_ID }
        val normalizedName = normalizeListName(name)
        require(normalizedName.isNotBlank()) { "Todo list name is required" }
        val catalog = loadLists(now)
        require(!listNameTaken(catalog, normalizedName, normalizedId)) { "Todo list \"$normalizedName\" already exists" }
        var found = false
        val nextLists = catalog.lists.map { meta ->
            if (meta.id != normalizedId || meta.deleted) return@map meta
            found = true
            meta.copy(
                name = normalizedName,
                rev = revForTime(now),
                updatedAt = now.withOffsetSameInstant(ZoneOffset.UTC),
            )
        }
        require(found) { "Todo list \"$normalizedId\" does not exist" }
        val next = catalog.copy(lists = nextLists)
        saveLists(next, now)
        return loadLists(now)
    }

    fun deleteList(id: String, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoListsStore {
        val normalizedId = normalizeListId(id)
        require(normalizedId != DEFAULT_LIST_ID) { "Default todo list cannot be deleted" }
        val catalog = loadLists(now)
        var found = false
        val utcNow = now.withOffsetSameInstant(ZoneOffset.UTC)
        val nextLists = catalog.lists.map { meta ->
            if (meta.id != normalizedId) return@map meta
            found = true
            meta.copy(
                rev = revForTime(utcNow),
                updatedAt = utcNow,
                deleted = true,
                deletedAt = utcNow,
            )
        }
        require(found) { "Todo list \"$normalizedId\" does not exist" }
        if (currentListId() == normalizedId) currentListId = DEFAULT_LIST_ID
        saveLists(catalog.copy(lists = nextLists), utcNow)
        runCatching { storePath(normalizedId).delete() }
        return loadLists(utcNow)
    }

    fun removeDeletedListData(catalog: TodoListsStore) {
        catalog.lists
            .filter { it.deleted && it.id != DEFAULT_LIST_ID }
            .forEach { runCatching { storePath(it.id).delete() } }
    }

    fun currentList(now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoListMeta {
        val catalog = loadLists(now)
        return activeLists(catalog).firstOrNull { it.id == currentListId() } ?: defaultListMeta()
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
            term = TERM_SHORT,
            order = nextActiveOrder(store.items),
            createdAt = now,
            updatedAt = now,
        )
        return store.copy(items = store.items + item).also { save(it) }
    }

    fun toggle(id: String): TodoStore {
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val updated = store.items.map { item ->
            if (item.id != id) return@map item
            if (item.status == STATUS_TODO && item.checkedAt == null) {
                item.copy(checkedAt = now, updatedAt = now)
            } else {
                val term = normalizeTerm(item.term)
                item.copy(
                    status = STATUS_TODO,
                    term = term,
                    order = nextActiveOrderForTerm(store.items, term),
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
        val term = store.items.firstOrNull { it.id == id }?.term?.let { normalizeTerm(it) } ?: return store
        val active = activeItems(store).filter { it.checkedAt == null && normalizeTerm(it.term) == term }
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

    fun moveTerm(id: String): TodoStore {
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val store = load(now)
        val updated = moveActiveToOtherTerm(store, id, now)
        if (updated == store) return store
        save(updated)
        return updated
    }

    fun todoCount(id: String): Int = loadList(id).items.size

    private fun storePath(id: String): File {
        val normalizedId = normalizeListId(id).ifBlank { DEFAULT_LIST_ID }
        return if (normalizedId == DEFAULT_LIST_ID) {
            todosPath()
        } else {
            File(File(context.filesDir, NAMED_LISTS_DIR), "$normalizedId.json")
        }
    }

    private fun generateUniqueListId(catalog: TodoListsStore): String {
        repeat(32) {
            val id = normalizeListId(idGenerator())
            if (id.isNotBlank() && id != DEFAULT_LIST_ID && catalog.lists.none { it.id == id }) {
                return id
            }
        }
        error("Could not allocate unique todo list id")
    }

    companion object {
        const val TODOS_FILE = "todos.json"
        const val LISTS_FILE = "todo_lists.json"
        const val NAMED_LISTS_DIR = "todo-lists"
        const val DEFAULT_LIST_ID = "default"
        const val DEFAULT_LIST_NAME = "Default"
        const val DEFAULT_LIST_REV = 1L
        const val SCHEMA_VERSION = 1
        const val STATUS_TODO = "todo"
        const val STATUS_DONE = "done"
        const val STATUS_ARCHIVED = "archived"
        const val TERM_SHORT = "short"
        const val TERM_LONG = "long"
        const val CHECKED_DELAY_SECONDS = 10L
        const val ARCHIVE_AFTER_DAYS = 7L

        fun defaultListMeta(): TodoListMeta {
            val utc = defaultListBaselineTime()
            return TodoListMeta(
                id = DEFAULT_LIST_ID,
                name = DEFAULT_LIST_NAME,
                rev = DEFAULT_LIST_REV,
                createdAt = utc,
                updatedAt = utc,
            )
        }

        fun defaultListBaselineTime(): OffsetDateTime {
            return OffsetDateTime.ofInstant(Instant.ofEpochMilli(DEFAULT_LIST_REV), ZoneOffset.UTC)
        }

        fun normalizeLists(catalog: TodoListsStore, now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC)): TodoListsStore {
            val utcNow = now.withOffsetSameInstant(ZoneOffset.UTC)
            val byId = linkedMapOf<String, TodoListMeta>()
            catalog.lists.forEach { raw ->
                val id = normalizeListId(raw.id)
                if (id.isBlank()) return@forEach
                val name = normalizeListName(raw.name).ifBlank {
                    if (id == DEFAULT_LIST_ID) DEFAULT_LIST_NAME else id
                }
                val createdAt = raw.createdAt.withOffsetSameInstant(ZoneOffset.UTC)
                val updatedAt = raw.updatedAt.withOffsetSameInstant(ZoneOffset.UTC)
                val normalized = raw.copy(
                    id = id,
                    name = name,
                    rev = raw.rev.takeIf { it > 0L } ?: revForTime(updatedAt),
                    createdAt = createdAt,
                    updatedAt = updatedAt,
                    deletedAt = raw.deletedAt?.withOffsetSameInstant(ZoneOffset.UTC),
                )
                val existing = byId[id]
                if (existing == null || listMetaWins(normalized, existing)) {
                    byId[id] = normalized
                }
            }
            val default = (byId[DEFAULT_LIST_ID] ?: defaultListMeta()).let {
                it.copy(
                    id = DEFAULT_LIST_ID,
                    name = normalizeListName(it.name).ifBlank { DEFAULT_LIST_NAME },
                    deleted = false,
                    deletedAt = null,
                    rev = it.rev.takeIf { rev -> rev > 0L } ?: DEFAULT_LIST_REV,
                )
            }
            byId[DEFAULT_LIST_ID] = default
            val active = byId.values
                .filter { it.id != DEFAULT_LIST_ID && !it.deleted }
                .sortedWith(compareBy<TodoListMeta> { it.name.lowercase() }.thenBy { it.name }.thenBy { it.id })
            val tombstones = byId.values
                .filter { it.id != DEFAULT_LIST_ID && it.deleted }
                .sortedWith(compareByDescending<TodoListMeta> { it.updatedAt }.thenBy { it.id })
            val lists = listOf(default) + active + tombstones
            val current = normalizeCurrentListId(catalog.currentListId)
                .takeIf { id -> lists.any { it.id == id && !it.deleted } }
                ?: DEFAULT_LIST_ID
            return TodoListsStore(version = SCHEMA_VERSION, currentListId = current, lists = lists)
        }

        fun activeLists(catalog: TodoListsStore): List<TodoListMeta> {
            return normalizeLists(catalog).lists.filterNot { it.deleted }
        }

        fun listById(catalog: TodoListsStore, id: String): TodoListMeta? {
            val normalizedId = normalizeListId(id).ifBlank { DEFAULT_LIST_ID }
            return normalizeLists(catalog).lists.firstOrNull { it.id == normalizedId }
        }

        fun mergeLists(
            local: TodoListsStore,
            remote: TodoListsStore,
            now: OffsetDateTime = OffsetDateTime.now(ZoneOffset.UTC),
        ): TodoListsStore {
            val normalizedLocal = normalizeLists(local, now)
            val normalizedRemote = normalizeLists(remote, now)
            val byId = linkedMapOf<String, TodoListMeta>()
            normalizedLocal.lists.forEach { byId[it.id] = it }
            normalizedRemote.lists.forEach { remoteMeta ->
                val localMeta = byId[remoteMeta.id]
                if (localMeta == null || listMetaWins(remoteMeta, localMeta)) {
                    byId[remoteMeta.id] = remoteMeta
                }
            }
            return normalizeLists(
                TodoListsStore(
                    currentListId = normalizedLocal.currentListId,
                    lists = byId.values.toList(),
                ),
                now,
            )
        }

        fun normalizeListName(name: String): String {
            return name.trim().split(Regex("\\s+")).filter { it.isNotBlank() }.joinToString(" ")
        }

        fun normalizeListId(id: String): String {
            val cleaned = id.trim().lowercase().replace('_', '-')
            val out = StringBuilder()
            var previousDash = false
            cleaned.forEach { ch ->
                val valid = ch in 'a'..'z' || ch in '0'..'9'
                if (valid) {
                    out.append(ch)
                    previousDash = false
                } else if (!previousDash) {
                    out.append('-')
                    previousDash = true
                }
            }
            return out.toString().trim('-')
        }

        fun normalizeCurrentListId(id: String): String {
            return normalizeListId(id).ifBlank { DEFAULT_LIST_ID }
        }

        fun parseListMeta(json: JSONObject): TodoListMeta {
            val id = normalizeListId(json.optString("id"))
            val now = OffsetDateTime.now(ZoneOffset.UTC)
            val fallback = if (id == DEFAULT_LIST_ID) defaultListBaselineTime() else now
            val created = parseTime(json.optString("created_at")) ?: fallback
            val updated = parseTime(json.optString("updated_at")) ?: created
            val rev = json.optLong("rev", 0L).takeIf { it > 0L }
                ?: if (id == DEFAULT_LIST_ID) DEFAULT_LIST_REV else revForTime(updated)
            return TodoListMeta(
                id = id,
                name = normalizeListName(json.optString("name")),
                rev = rev,
                createdAt = created,
                updatedAt = updated,
                deleted = json.optBoolean("deleted", false),
                deletedAt = parseTime(json.optString("deleted_at")),
            )
        }

        fun listMetaJson(meta: TodoListMeta): JSONObject {
            val json = JSONObject()
                .put("id", normalizeCurrentListId(meta.id))
                .put("name", normalizeListName(meta.name).ifBlank { DEFAULT_LIST_NAME })
                .put("rev", meta.rev)
                .put("created_at", formatTime(meta.createdAt))
                .put("updated_at", formatTime(meta.updatedAt))
                .put("deleted", meta.deleted)
            meta.deletedAt?.let { json.put("deleted_at", formatTime(it)) }
            return json
        }

        fun listNameTaken(catalog: TodoListsStore, name: String, exceptId: String): Boolean {
            val normalizedName = normalizeListName(name).lowercase()
            val normalizedExcept = normalizeListId(exceptId)
            return normalizeLists(catalog).lists.any {
                !it.deleted &&
                    it.id != normalizedExcept &&
                    normalizeListName(it.name).lowercase() == normalizedName
            }
        }

        fun revForTime(value: OffsetDateTime): Long {
            return value.withOffsetSameInstant(ZoneOffset.UTC).toInstant().toEpochMilli()
        }

        private fun listMetaWins(candidate: TodoListMeta, current: TodoListMeta): Boolean {
            val candidateRev = candidate.rev.takeIf { it > 0L } ?: revForTime(candidate.updatedAt)
            val currentRev = current.rev.takeIf { it > 0L } ?: revForTime(current.updatedAt)
            if (candidateRev != currentRev) return candidateRev > currentRev
            if (candidate.deleted != current.deleted) return candidate.deleted
            if (candidate.updatedAt != current.updatedAt) return candidate.updatedAt.isAfter(current.updatedAt)
            return candidate.id < current.id
        }

        fun activeItems(store: TodoStore): List<TodoItem> {
            return store.items
                .filter { it.status == STATUS_TODO }
                .sortedWith(activeComparator())
        }

        fun shortItems(store: TodoStore): List<TodoItem> {
            return store.items
                .filter { it.status == STATUS_TODO && normalizeTerm(it.term) == TERM_SHORT }
                .sortedWith(activeComparator())
        }

        fun longItems(store: TodoStore): List<TodoItem> {
            return store.items
                .filter { it.status == STATUS_TODO && normalizeTerm(it.term) == TERM_LONG }
                .sortedWith(activeComparator())
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
            return store.copy(
                items = store.items.map { it.copy(term = normalizeTerm(it.term)) },
                archiveMonths = normalizeArchiveMonths(store.archiveMonths + archiveGroups(store).keys),
            )
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
            val draggedItem = store.items.firstOrNull { it.id == draggedId } ?: return store
            val targetItem = store.items.firstOrNull { it.id == targetId } ?: return store
            if (normalizeTerm(draggedItem.term) != normalizeTerm(targetItem.term)) return store
            val active = activeItems(store).filter { it.checkedAt == null && normalizeTerm(it.term) == normalizeTerm(draggedItem.term) }
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
            return nextActiveOrderForTerm(items, TERM_SHORT)
        }

        private fun nextActiveOrderForTerm(items: List<TodoItem>, term: String): Int {
            val normalized = normalizeTerm(term)
            return items.filter { it.status == STATUS_TODO && it.checkedAt == null && normalizeTerm(it.term) == normalized }
                .maxOfOrNull { it.order + 1 } ?: 0
        }

        private fun moveActiveToOtherTerm(store: TodoStore, id: String, now: OffsetDateTime): TodoStore {
            val updated = store.items.map { item ->
                if (item.id != id || item.status != STATUS_TODO || item.checkedAt != null) return@map item
                val nextTerm = if (normalizeTerm(item.term) == TERM_LONG) TERM_SHORT else TERM_LONG
                item.copy(
                    term = nextTerm,
                    order = nextActiveOrderForTerm(store.items, nextTerm),
                    updatedAt = now,
                )
            }
            return store.copy(items = updated)
        }

        private fun activeComparator(): Comparator<TodoItem> {
            return compareBy<TodoItem> { if (normalizeTerm(it.term) == TERM_LONG) 1 else 0 }
                .thenBy { it.checkedAt != null }
                .thenBy { it.order }
                .thenBy { it.createdAt }
        }

        fun normalizeTerm(term: String): String {
            return when (term.trim().lowercase()) {
                TERM_LONG -> TERM_LONG
                else -> TERM_SHORT
            }
        }

        fun parseItem(json: JSONObject): TodoItem {
            val created = parseTime(json.optString("created_at")) ?: OffsetDateTime.now(ZoneOffset.UTC)
            return TodoItem(
                id = json.optString("id"),
                text = json.optString("text"),
                status = json.optString("status", STATUS_TODO),
                term = normalizeTerm(json.optString("term", TERM_SHORT)),
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
                .put("term", normalizeTerm(item.term))
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

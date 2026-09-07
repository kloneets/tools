package com.kloneets.kokotools

import android.content.Context
import android.content.SharedPreferences
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URLEncoder
import java.net.URL
import java.security.KeyStore
import java.security.MessageDigest
import java.time.OffsetDateTime
import java.time.ZoneOffset
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

data class FirebaseSession(
    val uid: String,
    val email: String,
    val idToken: String,
    val refreshToken: String,
)

data class FirebaseRemoteNote(
    val id: String,
    val path: String,
    val text: String,
    val rev: Long,
    val deleted: Boolean,
)

data class FirebaseRemoteTodo(
    val item: TodoItem,
    val rev: Long,
    val deleted: Boolean,
)

data class FirebasePullResult(
    val todos: TodoStore,
    val todoChanged: Boolean,
    val remoteNotes: List<FirebaseRemoteNote>,
    val remoteTodoCount: Int,
    val remoteNoteCount: Int,
)

data class FirebaseSharedSettings(
    val values: JSONObject,
    val rev: Long,
)

data class FirebaseSyncHash(
    val hash: String,
    val updatedAt: String,
    val updatedBy: String,
)

internal class FirebaseNoteHistoryWriter(
    private val getRecord: (String) -> FirebaseVersionedNote,
    private val putHistory: (String, JSONObject) -> Unit,
    private val putCurrent: (String, JSONObject, String) -> Unit,
) {
    fun replace(workspacePath: String, noteId: String, newRecord: JSONObject) {
        val notePath = "$workspacePath/notes/$noteId"
        val current = getRecord(notePath)
        if (current.record != null) {
            val currentRevision = current.record.optLong("rev", 0L)
            putHistory("$workspacePath/note_versions/$noteId/$currentRevision", current.record)
            val proposedRevision = newRecord.optLong("rev", 0L)
            if (proposedRevision <= currentRevision && currentRevision < Long.MAX_VALUE) {
                newRecord.put("rev", currentRevision + 1L)
            }
        }
        putCurrent(notePath, newRecord, current.etag)
    }
}

internal data class FirebaseVersionedNote(
    val record: JSONObject?,
    val etag: String,
) {
    init {
        require(etag.isNotBlank()) { "Firebase note ETag is required" }
    }
}

internal data class FirebaseVersionedTodoListMeta(
    val record: TodoListMeta?,
    val etag: String,
) {
    init {
        require(etag.isNotBlank()) { "Firebase todo list ETag is required" }
    }
}

internal fun requireFirebaseNoteEtag(etag: String?): String {
    return etag?.takeIf { it.isNotBlank() }
        ?: throw IllegalStateException("Firebase note GET response did not include an ETag")
}

class FirebaseSyncRepository(private val context: Context) {
    private val tokenFile: File
        get() = File(context.filesDir, TOKEN_FILE)
    private val tokenStore = EncryptedFirebaseTokenStore(context) { tokenFile }
    private val syncState: SharedPreferences
        get() = context.getSharedPreferences(SYNC_STATE_PREFERENCES, Context.MODE_PRIVATE)

    fun backendConfigured(settings: FirebaseSettings): Boolean {
        return backendConfigReady(settings)
    }

    fun configured(settings: FirebaseSettings): Boolean {
        return backendConfigured(settings) && settings.workspaceId.isNotBlank()
    }

    fun hasSavedSession(): Boolean {
        return tokenStore.load() != null
    }

    fun clearSavedSession() {
        tokenStore.clear()
    }

    fun currentSession(settings: FirebaseSettings): FirebaseSession? {
        val saved = loadToken() ?: return null
        return refresh(settings, saved.refreshToken).getOrNull()
    }

    fun login(settings: FirebaseSettings, email: String, password: String): FirebaseSession {
        return authenticate(settings, email, password, signUp = false)
    }

    fun register(settings: FirebaseSettings, email: String, password: String): FirebaseSession {
        return authenticate(settings, email, password, signUp = true)
    }

    fun loginWithGoogleIdToken(settings: FirebaseSettings, googleIdToken: String): FirebaseSession {
        require(googleIdToken.isNotBlank()) { "Google ID token is required" }
        val body = JSONObject()
            .put("postBody", googleSignInPostBody(googleIdToken))
            .put("requestUri", "http://localhost")
            .put("returnSecureToken", true)
        val response = postJson(
            "https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${encode(settings.apiKey)}",
            body,
        )
        val session = FirebaseSession(
            uid = response.getString("localId"),
            email = response.optString("email", ""),
            idToken = response.getString("idToken"),
            refreshToken = response.getString("refreshToken"),
        )
        saveToken(session)
        return session
    }

    fun ensurePersonalWorkspace(settings: FirebaseSettings, session: FirebaseSession): FirebaseSettings {
        val personalWorkspaceId = personalWorkspaceId(session.uid)
        if (settings.workspaceId.isNotBlank() && settings.workspaceId != personalWorkspaceId && !settings.workspaceId.startsWith("user_")) {
            return settings
        }
        val workspaceId = personalWorkspaceId
        val workspaceName = settings.workspaceName.ifBlank { "Personal workspace" }
        val workspaceSettings = settings.copy(workspaceId = workspaceId, workspaceName = workspaceName)
        val member = JSONObject()
            .put("email", session.email)
            .put("role", "owner")
            .put("joined_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
        putDatabase(workspaceSettings, "workspaces/$workspaceId/members/${session.uid}", member, session.idToken)
        val meta = JSONObject()
            .put("name", workspaceName)
            .put("owner", session.uid)
            .put("created_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
        putDatabase(workspaceSettings, "workspaces/$workspaceId/meta", meta, session.idToken)
        return workspaceSettings
    }

    private fun authenticate(settings: FirebaseSettings, email: String, password: String, signUp: Boolean): FirebaseSession {
        val body = JSONObject()
            .put("email", email)
            .put("password", password)
            .put("returnSecureToken", true)
        val endpoint = if (signUp) "accounts:signUp" else "accounts:signInWithPassword"
        val response = postJson(
            "https://identitytoolkit.googleapis.com/v1/$endpoint?key=${encode(settings.apiKey)}",
            body,
        )
        val session = FirebaseSession(
            uid = response.getString("localId"),
            email = response.optString("email", email),
            idToken = response.getString("idToken"),
            refreshToken = response.getString("refreshToken"),
        )
        saveToken(session)
        return session
    }

    fun refresh(settings: FirebaseSettings, refreshToken: String): Result<FirebaseSession> {
        return runCatching {
            val form = "grant_type=refresh_token&refresh_token=${encode(refreshToken)}"
            val response = requestJson(
                "POST",
                "https://securetoken.googleapis.com/v1/token?key=${encode(settings.apiKey)}",
                form,
                "application/x-www-form-urlencoded",
            )
            val previous = loadToken()
            val session = FirebaseSession(
                uid = response.getString("user_id"),
                email = previous?.email.orEmpty(),
                idToken = response.getString("id_token"),
                refreshToken = response.getString("refresh_token"),
            )
            saveToken(session)
            session
        }
    }

    fun pushTodos(
        settings: FirebaseSettings,
        store: TodoStore,
        session: FirebaseSession,
        listId: String = TodoRepository.DEFAULT_LIST_ID,
    ) {
        val normalizedListId = normalizeTodoListIdForSync(listId)
        store.items.forEach { item ->
            val rev = item.updatedAt.toInstant().toEpochMilli()
            if (item.status == TodoRepository.STATUS_ARCHIVED) {
                val record = JSONObject()
                    .put("item", TodoRepository.itemJson(item.copy(text = "")))
                    .put("rev", rev)
                    .put("updated_by", session.uid)
                    .put("deleted", true)
                putDatabase(settings, todoRecordPath(settings.workspaceId, normalizedListId, item.id), record, session.idToken)
                return@forEach
            }
            val record = JSONObject()
                .put("item", TodoRepository.itemJson(item))
                .put("rev", rev)
                .put("updated_by", session.uid)
                .put("deleted", false)
            putDatabase(settings, todoRecordPath(settings.workspaceId, normalizedListId, item.id), record, session.idToken)
        }
        runCatching {
            putDatabaseBestEffort(
                settings,
                todoArchiveMonthsPath(settings.workspaceId, normalizedListId),
                org.json.JSONArray().apply { TodoRepository.archiveMonths(store).forEach { put(it) } }.toString(),
                session.idToken,
            )
        }
        val now = TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC))
        val todosFeature = todoFeature(normalizedListId)
        val monthsFeature = todoArchiveMonthsFeature(normalizedListId)
        writeSyncHashBestEffort(settings, todosFeature, todoStoreHash(store), now, session)
        writeSyncHashBestEffort(settings, monthsFeature, todoArchiveMonthsHash(TodoRepository.archiveMonths(store)), now, session)
    }

    @Synchronized
    fun pushNote(settings: FirebaseSettings, path: String, text: String, session: FirebaseSession) {
        val normalized = NotesRepository.normalizePath(path).replace('\\', '/')
        val rev = System.currentTimeMillis()
        val id = noteId(normalized)
        val record = JSONObject()
            .put("id", id)
            .put("path", normalized)
            .put("text", text)
            .put("rev", rev)
            .put("updated_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
            .put("updated_by", session.uid)
            .put("deleted", false)
        replaceNoteRecord(settings, id, record, session.idToken)
    }

    @Synchronized
    fun pushNoteDelete(settings: FirebaseSettings, path: String, session: FirebaseSession) {
        val normalized = NotesRepository.normalizePath(path).replace('\\', '/')
        val rev = System.currentTimeMillis()
        val id = noteId(normalized)
        val record = JSONObject()
            .put("id", id)
            .put("path", normalized)
            .put("text", "")
            .put("rev", rev)
            .put("updated_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
            .put("updated_by", session.uid)
            .put("deleted", true)
        replaceNoteRecord(settings, id, record, session.idToken)
    }

    fun pullNotes(settings: FirebaseSettings, session: FirebaseSession): List<FirebaseRemoteNote> {
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/notes", session.idToken)
        return parseRemoteNotes(remote)
    }

    fun pullTodos(
        settings: FirebaseSettings,
        local: TodoStore,
        session: FirebaseSession,
        forceFull: Boolean = false,
        listId: String = TodoRepository.DEFAULT_LIST_ID,
        replaceLocal: Boolean = false,
    ): TodoStore {
        val normalizedListId = normalizeTodoListIdForSync(listId)
        val hashes = syncHashes(settings, session)
        val todosFeature = todoFeature(normalizedListId)
        val monthsFeature = todoArchiveMonthsFeature(normalizedListId)
        val skipTodos = !forceFull && !replaceLocal && hashes[todosFeature]?.let { shouldSkipFeature(settings, todosFeature, it) } == true
        val skipMonths = !forceFull && !replaceLocal && hashes[monthsFeature]?.let { shouldSkipFeature(settings, monthsFeature, it) } == true
        val remoteItems = if (skipTodos) emptyList() else {
            pullRemoteTodos(settings, session, normalizedListId)
                .filter { it.deleted || it.item.status != TodoRepository.STATUS_ARCHIVED }
        }
        val remoteArchiveMonths = if (skipMonths) emptyList() else pullTodoArchiveMonths(settings, session, normalizedListId)
        val merged = mergeTodoRecords(local, remoteItems, replaceLocal = replaceLocal)
        val archiveMonths = if (replaceLocal && !skipMonths) remoteArchiveMonths else merged.archiveMonths + remoteArchiveMonths
        val normalized = TodoRepository.normalize(merged.copy(archiveMonths = archiveMonths))
        if (!skipTodos) {
            val hash = remoteTodoHash(remoteItems)
            markFeaturePulled(settings, todosFeature, hash)
            writeSyncHashBestEffort(settings, todosFeature, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        }
        if (!skipMonths) {
            val hash = todoArchiveMonthsHash(remoteArchiveMonths)
            markFeaturePulled(settings, monthsFeature, hash)
            writeSyncHashBestEffort(settings, monthsFeature, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        }
        return normalized
    }

    fun pullRemoteTodoStore(
        settings: FirebaseSettings,
        session: FirebaseSession,
        listId: String = TodoRepository.DEFAULT_LIST_ID,
    ): TodoStore {
        return TodoStore(
            items = sortedTodos(
                pullRemoteTodos(settings, session, listId)
                    .filterNot { it.deleted }
                    .map { it.item }
                    .filter { it.status != TodoRepository.STATUS_ARCHIVED },
            ),
        )
    }

    fun pullTodoArchiveMonth(
        settings: FirebaseSettings,
        month: String,
        session: FirebaseSession,
        listId: String = TodoRepository.DEFAULT_LIST_ID,
    ): List<TodoItem> {
        val normalizedListId = normalizeTodoListIdForSync(listId)
        val feature = todoArchiveMonthFeature(normalizedListId, month)
        syncHashes(settings, session)[feature]?.let { remote ->
            if (shouldSkipFeature(settings, feature, remote)) return emptyList()
        }
        val remote = getDatabase(settings, todoArchiveMonthPath(settings.workspaceId, normalizedListId, month), session.idToken)
        val records = mutableListOf<TodoItem>()
        if (remote != null) {
            remote.keys().forEach { id ->
                val record = remote.optJSONObject(id) ?: return@forEach
                if (record.optBoolean("deleted", false)) return@forEach
                val itemJson = record.optJSONObject("item") ?: return@forEach
                val item = TodoRepository.parseItem(itemJson)
                if (item.status == TodoRepository.STATUS_ARCHIVED &&
                    item.archivedAt != null &&
                    item.archivedAt.format(java.time.format.DateTimeFormatter.ofPattern("yyyy-MM")) == month
                ) {
                    records += item
                }
            }
        }
        val sorted = records.sortedByDescending { it.archivedAt }
        val hash = todoArchiveMonthHash(sorted)
        markFeaturePulled(settings, feature, hash)
        writeSyncHashBestEffort(settings, feature, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        return sorted
    }

    fun pullTodoArchiveMonths(
        settings: FirebaseSettings,
        session: FirebaseSession,
        listId: String = TodoRepository.DEFAULT_LIST_ID,
    ): List<String> {
        val response = request("GET", databaseUrl(settings, todoArchiveMonthsPath(settings.workspaceId, normalizeTodoListIdForSync(listId)), session.idToken), null, null)
        if (response.isBlank() || response == "null") return emptyList()
        val values = org.json.JSONArray(response)
        return (0 until values.length()).mapNotNull { index ->
            values.optString(index).takeIf { it.isNotBlank() }
        }
    }

    fun pullTodoLists(settings: FirebaseSettings, session: FirebaseSession): TodoListsStore {
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/todo_lists", session.idToken)
        val lists = mutableListOf<TodoListMeta>()
        remote?.keys()?.forEach { fallbackId ->
            val meta = remote.optJSONObject(fallbackId)?.optJSONObject("meta") ?: return@forEach
            val parsed = TodoRepository.parseListMeta(meta)
            lists += if (parsed.id.isBlank()) parsed.copy(id = fallbackId) else parsed
        }
        val catalog = TodoRepository.normalizeLists(TodoListsStore(lists = lists))
        val now = TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC))
        val hash = todoListsHash(catalog)
        markFeaturePulled(settings, SYNC_FEATURE_TODO_LISTS, hash)
        writeSyncHashBestEffort(settings, SYNC_FEATURE_TODO_LISTS, hash, now, session)
        return catalog
    }

    fun pushTodoLists(settings: FirebaseSettings, catalog: TodoListsStore, session: FirebaseSession): Boolean {
        val normalized = TodoRepository.normalizeLists(catalog)
        var stale = false
        normalized.lists.forEach { meta ->
            val wrote = putTodoListMetaIfCurrent(settings, meta, session)
            if (!wrote) {
                stale = true
                return@forEach
            }
            if (meta.deleted && meta.id != TodoRepository.DEFAULT_LIST_ID) {
                deleteTodoListDataBestEffort(settings, meta.id, session)
            }
        }
        if (stale) return false
        val hash = todoListsHash(normalized)
        markFeaturePulled(settings, SYNC_FEATURE_TODO_LISTS, hash)
        writeSyncHashBestEffort(settings, SYNC_FEATURE_TODO_LISTS, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        return true
    }

    fun deleteTodoListDataBestEffort(settings: FirebaseSettings, listId: String, session: FirebaseSession) {
        val normalizedListId = normalizeTodoListIdForSync(listId)
        if (normalizedListId == TodoRepository.DEFAULT_LIST_ID) return
        listOf("todos", "archive_months", "archives").forEach { child ->
            runCatching {
                request("DELETE", databaseUrl(settings, todoListChildPath(settings.workspaceId, normalizedListId, child), session.idToken), null, null)
            }
        }
    }

    private fun putTodoListMetaIfCurrent(
        settings: FirebaseSettings,
        meta: TodoListMeta,
        session: FirebaseSession,
    ): Boolean {
        val path = todoListMetaPath(settings.workspaceId, meta.id)
        val versioned = getVersionedTodoListMeta(settings, path, session.idToken)
        if (versioned.record != null && !shouldWriteTodoListMeta(meta, versioned.record)) {
            return false
        }
        return runCatching {
            putDatabaseIfMatch(settings, path, TodoRepository.listMetaJson(meta), session.idToken, versioned.etag)
            true
        }.getOrElse { error ->
            if (isFirebasePreconditionFailure(error)) false else throw error
        }
    }

    fun pushSharedSettings(settings: FirebaseSettings, appSettings: AppSettings, session: FirebaseSession): Boolean {
        val values = SettingsRepository.sharedSettingsJson(appSettings)
        val hash = sharedSettingsHash(values)
        if (lastSharedSettingsHash(settings) == hash) return false
        val now = OffsetDateTime.now(ZoneOffset.UTC)
        val rev = System.currentTimeMillis()
        val record = JSONObject()
            .put("values", values)
            .put("rev", rev)
            .put("updated_at", TodoRepository.formatTime(now))
            .put("updated_by", session.uid)
        putDatabase(settings, "workspaces/${settings.workspaceId}/settings/shared", record, session.idToken)
        markSharedSettingsSynced(settings, values)
        markFeaturePulled(settings, SYNC_FEATURE_SETTINGS, hash)
        writeSyncHashBestEffort(settings, SYNC_FEATURE_SETTINGS, hash, TodoRepository.formatTime(now), session)
        return true
    }

    fun pullSharedSettings(settings: FirebaseSettings, session: FirebaseSession): FirebaseSharedSettings? {
        syncHashes(settings, session)[SYNC_FEATURE_SETTINGS]?.let { remote ->
            if (shouldSkipFeature(settings, SYNC_FEATURE_SETTINGS, remote)) return null
        }
        val record = getDatabase(settings, "workspaces/${settings.workspaceId}/settings/shared", session.idToken) ?: return null
        val values = record.optJSONObject("values") ?: return null
        val hash = sharedSettingsHash(values)
        markFeaturePulled(settings, SYNC_FEATURE_SETTINGS, hash)
        writeSyncHashBestEffort(settings, SYNC_FEATURE_SETTINGS, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        return FirebaseSharedSettings(values = values, rev = record.optLong("rev", 0L))
    }

    fun markSharedSettingsSynced(settings: FirebaseSettings, values: JSONObject) {
        syncState.edit()
            .putString(sharedSettingsHashKey(settings), sharedSettingsHash(values))
            .apply()
    }

    fun localSharedSettingsHash(settings: AppSettings): String {
        return sharedSettingsHash(SettingsRepository.sharedSettingsJson(settings))
    }

    private fun lastSharedSettingsHash(settings: FirebaseSettings): String? {
        return syncState.getString(sharedSettingsHashKey(settings), null)
    }

    fun deleteLegacyAssetsBestEffort(settings: FirebaseSettings, session: FirebaseSession) {
        runCatching {
            request("DELETE", databaseUrl(settings, "workspaces/${settings.workspaceId}/assets", session.idToken), null, null)
        }
        runCatching {
            request("DELETE", databaseUrl(settings, "workspaces/${settings.workspaceId}/sync_hashes/assets", session.idToken), null, null)
        }
    }

    private fun pullRemoteTodos(
        settings: FirebaseSettings,
        session: FirebaseSession,
        listId: String = TodoRepository.DEFAULT_LIST_ID,
    ): List<FirebaseRemoteTodo> {
        val remote = getDatabase(settings, todoTodosPath(settings.workspaceId, normalizeTodoListIdForSync(listId)), session.idToken)
        val records = mutableListOf<FirebaseRemoteTodo>()
        if (remote != null) {
            remote.keys().forEach { id ->
                val record = remote.optJSONObject(id) ?: return@forEach
                val itemJson = record.optJSONObject("item") ?: return@forEach
                val item = TodoRepository.parseItem(itemJson)
                records += FirebaseRemoteTodo(
                    item = item,
                    rev = record.optLong("rev", 0L),
                    deleted = record.optBoolean("deleted", false),
                )
            }
        }
        return records
    }

    private fun sortedTodos(items: Collection<TodoItem>): List<TodoItem> {
        return items.sortedWith(compareBy<TodoItem> { it.status }.thenBy { it.order }.thenBy { it.createdAt })
    }

    private fun loadToken(): FirebaseSession? {
        return tokenStore.load()
    }

    private fun saveToken(session: FirebaseSession) {
        tokenStore.save(session)
    }

    private fun postJson(url: String, body: JSONObject): JSONObject {
        return requestJson("POST", url, body.toString(), "application/json")
    }

    private fun putDatabase(settings: FirebaseSettings, path: String, body: JSONObject, idToken: String) {
        runCatching {
            requestJson("PUT", databaseUrl(settings, path, idToken), body.toString(), "application/json")
        }.getOrElse { error ->
            throw IllegalStateException("Firebase PUT $path failed: ${error.message}", error)
        }
    }

    private fun putDatabase(settings: FirebaseSettings, path: String, body: String, idToken: String) {
        runCatching {
            requestJson("PUT", databaseUrl(settings, path, idToken), body, "application/json")
        }.getOrElse { error ->
            throw IllegalStateException("Firebase PUT $path failed: ${error.message}", error)
        }
    }

    private fun putDatabaseBestEffort(settings: FirebaseSettings, path: String, body: String, idToken: String) {
        requestJson("PUT", databaseUrl(settings, path, idToken), body, "application/json")
    }

    private fun replaceNoteRecord(settings: FirebaseSettings, id: String, record: JSONObject, idToken: String) {
        FirebaseNoteHistoryWriter(
            getRecord = { path -> getVersionedNote(settings, path, idToken) },
            putHistory = { path, body -> putDatabase(settings, path, body, idToken) },
            putCurrent = { path, body, etag -> putDatabaseIfMatch(settings, path, body, idToken, etag) },
        ).replace("workspaces/${settings.workspaceId}", id, record)
    }

    private fun getVersionedNote(settings: FirebaseSettings, path: String, idToken: String): FirebaseVersionedNote {
        val response = runCatching {
            requestWithMetadata(
                method = "GET",
                url = databaseUrl(settings, path, idToken),
                body = null,
                contentType = null,
                headers = mapOf("X-Firebase-ETag" to "true"),
            )
        }.getOrElse { error ->
            throw IllegalStateException("Firebase GET $path failed: ${error.message}", error)
        }
        val record = response.body.takeUnless { it.isBlank() || it == "null" }?.let(::JSONObject)
        return FirebaseVersionedNote(record = record, etag = requireFirebaseNoteEtag(response.etag))
    }

    private fun getVersionedTodoListMeta(
        settings: FirebaseSettings,
        path: String,
        idToken: String,
    ): FirebaseVersionedTodoListMeta {
        val response = runCatching {
            requestWithMetadata(
                method = "GET",
                url = databaseUrl(settings, path, idToken),
                body = null,
                contentType = null,
                headers = mapOf("X-Firebase-ETag" to "true"),
            )
        }.getOrElse { error ->
            throw IllegalStateException("Firebase GET $path failed: ${error.message}", error)
        }
        val record = response.body
            .takeUnless { it.isBlank() || it == "null" }
            ?.let { TodoRepository.parseListMeta(JSONObject(it)) }
        return FirebaseVersionedTodoListMeta(record = record, etag = requireFirebaseNoteEtag(response.etag))
    }

    private fun putDatabaseIfMatch(
        settings: FirebaseSettings,
        path: String,
        body: JSONObject,
        idToken: String,
        etag: String,
    ) {
        runCatching {
            requestJson(
                method = "PUT",
                url = databaseUrl(settings, path, idToken),
                body = body.toString(),
                contentType = "application/json",
                headers = mapOf("If-Match" to etag),
            )
        }.getOrElse { error ->
            throw IllegalStateException("Firebase conditional PUT $path failed: ${error.message}", error)
        }
    }

    private fun isFirebasePreconditionFailure(error: Throwable): Boolean {
        return error.message?.contains("412") == true ||
            error.message?.contains("Precondition Failed", ignoreCase = true) == true
    }

    private fun getDatabase(settings: FirebaseSettings, path: String, idToken: String): JSONObject? {
        val response = runCatching {
            request("GET", databaseUrl(settings, path, idToken), null, null)
        }.getOrElse { error ->
            throw IllegalStateException("Firebase GET $path failed: ${error.message}", error)
        }
        if (response.isBlank() || response == "null") return null
        return JSONObject(response)
    }

    private fun syncHashes(settings: FirebaseSettings, session: FirebaseSession): Map<String, FirebaseSyncHash> {
        return runCatching {
            val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/sync_hashes", session.idToken) ?: return emptyMap()
            val hashes = mutableMapOf<String, FirebaseSyncHash>()
            remote.keys().forEach { feature ->
                val record = remote.optJSONObject(feature) ?: return@forEach
                val hash = record.optString("hash", "")
                if (hash.isBlank()) return@forEach
                hashes[feature] = FirebaseSyncHash(
                    hash = hash,
                    updatedAt = record.optString("updated_at", ""),
                    updatedBy = record.optString("updated_by", ""),
                )
            }
            hashes
        }.getOrDefault(emptyMap())
    }

    private fun writeSyncHashBestEffort(settings: FirebaseSettings, feature: String, hash: String, updatedAt: String, session: FirebaseSession) {
        if (hash.isBlank()) return
        runCatching {
            val record = JSONObject()
                .put("hash", hash)
                .put("updated_at", updatedAt)
                .put("updated_by", session.uid)
            putDatabase(settings, "workspaces/${settings.workspaceId}/sync_hashes/${encodePath(feature)}", record, session.idToken)
        }
    }

    private fun shouldSkipFeature(settings: FirebaseSettings, feature: String, remote: FirebaseSyncHash): Boolean {
        val hashKey = syncHashKey(settings, feature)
        if (syncState.getString(hashKey, null) != remote.hash) return false
        val lastFullPull = syncState.getLong(syncHashPulledAtKey(settings, feature), 0L)
        if (lastFullPull <= 0L) return false
        return System.currentTimeMillis() - lastFullPull < SYNC_HASH_FULL_VALIDATION_MS
    }

    private fun markFeaturePulled(settings: FirebaseSettings, feature: String, hash: String) {
        if (hash.isBlank()) return
        syncState.edit()
            .putString(syncHashKey(settings, feature), hash)
            .putLong(syncHashPulledAtKey(settings, feature), System.currentTimeMillis())
            .apply()
    }

    private fun requestJson(
        method: String,
        url: String,
        body: String?,
        contentType: String,
        headers: Map<String, String> = emptyMap(),
    ): JSONObject {
        val response = request(method, url, body, contentType, headers)
        return JSONObject(response)
    }

    private fun request(
        method: String,
        url: String,
        body: String?,
        contentType: String?,
        headers: Map<String, String> = emptyMap(),
    ): String {
        return requestWithMetadata(method, url, body, contentType, headers).body
    }

    private fun requestWithMetadata(
        method: String,
        url: String,
        body: String?,
        contentType: String?,
        headers: Map<String, String> = emptyMap(),
    ): FirebaseHttpResponse {
        val connection = URL(url).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = method
            connection.connectTimeout = 15_000
            connection.readTimeout = 20_000
            headers.forEach(connection::setRequestProperty)
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", contentType ?: "application/json")
                connection.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            }
            val responseCode = connection.responseCode
            val stream = if (responseCode in 200..299) connection.inputStream else connection.errorStream
            val response = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
            if (responseCode !in 200..299) {
                throw IllegalStateException("Firebase request failed $responseCode: $response")
            }
            return FirebaseHttpResponse(body = response, etag = connection.getHeaderField("ETag"))
        } finally {
            connection.disconnect()
        }
    }

    private data class FirebaseHttpResponse(val body: String, val etag: String?)

    private fun databaseUrl(settings: FirebaseSettings, path: String, idToken: String): String {
        val baseUrl = settings.databaseUrl.trim()
        require(FirebaseConfigValidator.validDatabaseUrl(baseUrl)) { "Firebase database_url must be an absolute https URL with a host" }
        return baseUrl.trimEnd('/') + "/" + path.trim('/') + ".json?auth=${encode(idToken)}"
    }

    private fun encode(value: String): String {
        return URLEncoder.encode(value, Charsets.UTF_8.name())
    }

    private fun encodePath(value: String): String {
        return value.split("/").joinToString("/") { encode(it) }
    }

    private fun noteId(path: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(path.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }

    companion object {
        const val TOKEN_FILE = "firebase_token.json"
        private const val SYNC_STATE_PREFERENCES = "firebase_sync_state"
        private const val SYNC_FEATURE_TODOS = "todos"
        private const val SYNC_FEATURE_TODO_ARCHIVE_MONTHS = "todo_archive_months"
        private const val SYNC_FEATURE_TODO_LISTS = "todo_lists"
        private const val SYNC_FEATURE_TODO_ARCHIVE_MONTH_PREFIX = "todo_archive_month:"
        private const val SYNC_FEATURE_SETTINGS = "settings"
        private const val SYNC_HASH_FULL_VALIDATION_MS = 24L * 60L * 60L * 1000L

        fun personalWorkspaceId(uid: String): String = "user_$uid"

        fun backendConfigReady(settings: FirebaseSettings): Boolean {
            return settings.enabled &&
                settings.realtime &&
                settings.apiKey.isNotBlank() &&
                FirebaseConfigValidator.validDatabaseUrl(settings.databaseUrl)
        }

        fun mergeTodoRecords(
            local: TodoStore,
            remoteItems: List<FirebaseRemoteTodo>,
            replaceLocal: Boolean = false,
        ): TodoStore {
            val byId = if (replaceLocal) {
                mutableMapOf()
            } else {
                TodoRepository.nonArchivedStore(local).items.associateBy { it.id }.toMutableMap()
            }
            val localArchivedById = if (replaceLocal) {
                emptyMap()
            } else {
                local.items
                    .filter { it.status == TodoRepository.STATUS_ARCHIVED }
                    .associateBy { it.id }
            }
            remoteItems.forEach { record ->
                val item = record.item
                if (record.deleted) {
                    byId.remove(item.id)
                    return@forEach
                }
                val archivedLocal = localArchivedById[item.id]
                if (archivedLocal != null && !archivedLocal.updatedAt.isBefore(item.updatedAt)) {
                    return@forEach
                }
                val localItem = byId[item.id]
                if (localItem == null || !item.updatedAt.isBefore(localItem.updatedAt)) {
                    byId[item.id] = item
                }
            }
            val merged = local.copy(
                items = byId.values.sortedWith(compareBy<TodoItem> { it.status }.thenBy { it.order }.thenBy { it.createdAt }),
                archiveMonths = if (replaceLocal) emptyList() else local.archiveMonths,
            )
            return if (replaceLocal) {
                TodoRepository.normalize(merged)
            } else {
                TodoRepository.preserveArchived(local, merged)
            }
        }

        fun googleSignInPostBody(googleIdToken: String): String {
            return "id_token=${encodeQueryComponent(googleIdToken)}&providerId=${encodeQueryComponent("google.com")}"
        }

        fun sharedSettingsHash(values: JSONObject): String {
            val digest = MessageDigest.getInstance("SHA-256").digest(canonicalJson(values).toByteArray(Charsets.UTF_8))
            return digest.joinToString("") { "%02x".format(it) }
        }

        fun todoArchiveMonthsHash(months: List<String>): String {
            return sha256String(org.json.JSONArray().apply { months.sorted().forEach { put(it) } }.toString())
        }

        fun todoFeature(listId: String): String {
            val normalized = normalizeTodoListIdForSync(listId)
            return if (normalized == TodoRepository.DEFAULT_LIST_ID) {
                SYNC_FEATURE_TODOS
            } else {
                "todo_list:$normalized:todos"
            }
        }

        fun todoArchiveMonthsFeature(listId: String): String {
            val normalized = normalizeTodoListIdForSync(listId)
            return if (normalized == TodoRepository.DEFAULT_LIST_ID) {
                SYNC_FEATURE_TODO_ARCHIVE_MONTHS
            } else {
                "todo_list:$normalized:todo_archive_months"
            }
        }

        fun todoArchiveMonthFeature(listId: String, month: String): String {
            val normalized = normalizeTodoListIdForSync(listId)
            val normalizedMonth = month.trim()
            return if (normalized == TodoRepository.DEFAULT_LIST_ID) {
                "$SYNC_FEATURE_TODO_ARCHIVE_MONTH_PREFIX$normalizedMonth"
            } else {
                "todo_list:$normalized:todo_archive_month:$normalizedMonth"
            }
        }

        fun todoListsHash(catalog: TodoListsStore): String {
            val normalized = TodoRepository.normalizeLists(catalog)
            val values = normalized.lists.sortedBy { it.id }.joinToString(separator = ",", prefix = "[", postfix = "]") { meta ->
                """{"id":${JSONObject.quote(meta.id)},"name":${JSONObject.quote(meta.name)},"rev":${meta.rev},"deleted":${meta.deleted}}"""
            }
            return sha256String(values)
        }

        fun shouldWriteTodoListMeta(candidate: TodoListMeta, remote: TodoListMeta?): Boolean {
            if (remote == null) return true
            val candidateRev = todoListMetaRevision(candidate)
            val remoteRev = todoListMetaRevision(remote)
            if (remoteRev <= 0L) return true
            if (candidateRev != remoteRev) return candidateRev > remoteRev
            if (candidate.deleted != remote.deleted) return candidate.deleted
            return candidate.name == remote.name && candidate.deleted == remote.deleted
        }

        private fun todoListMetaRevision(meta: TodoListMeta): Long {
            return meta.rev.takeIf { it > 0L } ?: meta.updatedAt.toInstant().toEpochMilli()
        }

        fun todoStoreHash(store: TodoStore): String {
            return remoteTodoHash(store.items
                .map {
                    FirebaseRemoteTodo(
                        item = it,
                        rev = it.updatedAt.toInstant().toEpochMilli(),
                        deleted = it.status == TodoRepository.STATUS_ARCHIVED,
                    )
                })
        }

        fun remoteTodoHash(records: List<FirebaseRemoteTodo>): String {
            val values = records.sortedBy { it.item.id }.joinToString(separator = ",", prefix = "[", postfix = "]") { record ->
                """{"id":${JSONObject.quote(record.item.id)},"rev":${record.rev},"deleted":${record.deleted}}"""
            }
            return sha256String(values)
        }

        fun todoArchiveMonthHash(items: List<TodoItem>): String {
            val values = items.sortedBy { it.id }.joinToString(separator = ",", prefix = "[", postfix = "]") { item ->
                """{"id":${JSONObject.quote(item.id)},"rev":${item.updatedAt.toInstant().toEpochMilli()},"deleted":false}"""
            }
            return sha256String(values)
        }

        fun parseRemoteNotes(remote: JSONObject?): List<FirebaseRemoteNote> {
            if (remote == null) return emptyList()
            return remote.keys().asSequence().mapNotNull { fallbackId ->
                val record = remote.optJSONObject(fallbackId) ?: return@mapNotNull null
                val path = record.optString("path", "")
                if (path.isBlank()) return@mapNotNull null
                FirebaseRemoteNote(
                    id = record.optString("id", fallbackId).ifBlank { fallbackId },
                    path = NotesRepository.normalizePath(path).replace('\\', '/'),
                    text = record.optString("text", ""),
                    rev = record.optLong("rev", 0L),
                    deleted = record.optBoolean("deleted", false),
                )
            }.sortedWith(compareBy<FirebaseRemoteNote> { it.path.lowercase() }.thenBy { it.path }.thenBy { it.id }).toList()
        }

        private fun canonicalJson(value: Any?): String {
            return when (value) {
                null, JSONObject.NULL -> "null"
                is JSONObject -> {
                    val keys = value.keys().asSequence().toList().sorted()
                    keys.joinToString(separator = ",", prefix = "{", postfix = "}") { key ->
                        JSONObject.quote(key) + ":" + canonicalJson(value.opt(key))
                    }
                }
                is org.json.JSONArray -> {
                    (0 until value.length()).joinToString(separator = ",", prefix = "[", postfix = "]") { index ->
                        canonicalJson(value.opt(index))
                    }
                }
                is String -> JSONObject.quote(value)
                is Number, is Boolean -> value.toString()
                else -> JSONObject.quote(value.toString())
            }
        }

        private fun encodeQueryComponent(value: String): String {
            return URLEncoder.encode(value, Charsets.UTF_8.name())
        }

        private fun sha256String(value: String): String {
            val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
            return digest.joinToString("") { "%02x".format(it) }
        }

        fun todoTodosPath(workspaceId: String, listId: String): String {
            val normalized = normalizeTodoListIdForSync(listId)
            return if (normalized == TodoRepository.DEFAULT_LIST_ID) {
                "workspaces/$workspaceId/todos"
            } else {
                todoListChildPath(workspaceId, normalized, "todos")
            }
        }

        fun todoRecordPath(workspaceId: String, listId: String, todoId: String): String {
            return "${todoTodosPath(workspaceId, listId)}/$todoId"
        }

        fun todoArchiveMonthsPath(workspaceId: String, listId: String): String {
            val normalized = normalizeTodoListIdForSync(listId)
            return if (normalized == TodoRepository.DEFAULT_LIST_ID) {
                "workspaces/$workspaceId/todo_archive_months"
            } else {
                todoListChildPath(workspaceId, normalized, "archive_months")
            }
        }

        fun todoArchiveMonthPath(workspaceId: String, listId: String, month: String): String {
            val normalized = normalizeTodoListIdForSync(listId)
            return if (normalized == TodoRepository.DEFAULT_LIST_ID) {
                "workspaces/$workspaceId/todo_archives/${month.trim()}"
            } else {
                todoListChildPath(workspaceId, normalized, "archives/${month.trim()}")
            }
        }

        fun todoListMetaPath(workspaceId: String, listId: String): String {
            return todoListChildPath(workspaceId, normalizeTodoListIdForSync(listId), "meta")
        }

        fun todoListChildPath(workspaceId: String, listId: String, child: String): String {
            val suffix = child.trim('/')
            val base = "workspaces/$workspaceId/todo_lists/${normalizeTodoListIdForSync(listId)}"
            return if (suffix.isBlank()) base else "$base/$suffix"
        }

        fun normalizeTodoListIdForSync(listId: String): String {
            return TodoRepository.normalizeCurrentListId(listId)
        }

    }

    private fun sharedSettingsHashKey(settings: FirebaseSettings): String {
        return "shared_settings_hash:${settings.workspaceId.ifBlank { "default" }}"
    }

    private fun syncHashKey(settings: FirebaseSettings, feature: String): String {
        return "sync_hash:${settings.workspaceId.ifBlank { "default" }}:$feature"
    }

    private fun syncHashPulledAtKey(settings: FirebaseSettings, feature: String): String {
        return "sync_hash_pulled_at:${settings.workspaceId.ifBlank { "default" }}:$feature"
    }
}

private class EncryptedFirebaseTokenStore(
    private val context: Context,
    private val legacyTokenFile: () -> File,
) {
    private val preferences: SharedPreferences
        get() = context.getSharedPreferences(PREFERENCES_NAME, Context.MODE_PRIVATE)

    fun load(): FirebaseSession? {
        migrateLegacyToken()
        val payload = preferences.getString(KEY_PAYLOAD, null) ?: return null
        return runCatching {
            val json = JSONObject(decrypt(payload))
            FirebaseSession(
                uid = json.optString("uid"),
                email = json.optString("email"),
                idToken = "",
                refreshToken = json.optString("refresh_token"),
            )
        }.getOrNull()
    }

    fun save(session: FirebaseSession) {
        val json = JSONObject()
            .put("uid", session.uid)
            .put("email", session.email)
            .put("refresh_token", session.refreshToken)
        preferences.edit()
            .putString(KEY_PAYLOAD, encrypt(json.toString()))
            .apply()
    }

    fun clear() {
        preferences.edit().clear().apply()
        runCatching { legacyTokenFile().delete() }
    }

    private fun migrateLegacyToken() {
        if (preferences.contains(KEY_PAYLOAD)) return
        val legacy = legacyTokenFile()
        if (!legacy.isFile) return
        runCatching {
            val json = JSONObject(legacy.readText())
            val session = FirebaseSession(
                uid = json.optString("uid"),
                email = json.optString("email"),
                idToken = json.optString("id_token"),
                refreshToken = json.optString("refresh_token"),
            )
            if (session.refreshToken.isNotBlank()) save(session)
            legacy.delete()
        }
    }

    private fun encrypt(value: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, secretKey())
        val encrypted = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        return JSONObject()
            .put("iv", encodeBytes(cipher.iv))
            .put("data", encodeBytes(encrypted))
            .toString()
    }

    private fun decrypt(value: String): String {
        val json = JSONObject(value)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(
            Cipher.DECRYPT_MODE,
            secretKey(),
            GCMParameterSpec(128, decodeBytes(json.getString("iv"))),
        )
        return String(cipher.doFinal(decodeBytes(json.getString("data"))), Charsets.UTF_8)
    }

    private fun secretKey(): SecretKey {
        val keyStore = KeyStore.getInstance(ANDROID_KEY_STORE).apply { load(null) }
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEY_STORE)
        generator.init(
            KeyGenParameterSpec.Builder(
                KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true)
                .build(),
        )
        return generator.generateKey()
    }

    private fun encodeBytes(bytes: ByteArray): String = Base64.encodeToString(bytes, Base64.NO_WRAP)

    private fun decodeBytes(value: String): ByteArray = Base64.decode(value, Base64.NO_WRAP)

    companion object {
        private const val ANDROID_KEY_STORE = "AndroidKeyStore"
        private const val KEY_ALIAS = "koko_tools_firebase_refresh_token"
        private const val KEY_PAYLOAD = "token_payload"
        private const val PREFERENCES_NAME = "firebase_token_store"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"
    }
}

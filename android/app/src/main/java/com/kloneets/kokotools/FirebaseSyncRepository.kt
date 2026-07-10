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

class FirebaseSyncRepository(private val context: Context) {
    private val tokenFile: File
        get() = File(context.filesDir, TOKEN_FILE)
    private val tokenStore = EncryptedFirebaseTokenStore(context) { tokenFile }
    private val syncState: SharedPreferences
        get() = context.getSharedPreferences(SYNC_STATE_PREFERENCES, Context.MODE_PRIVATE)

    fun backendConfigured(settings: FirebaseSettings): Boolean {
        return settings.enabled &&
            settings.realtime &&
            settings.apiKey.isNotBlank() &&
            settings.databaseUrl.isNotBlank()
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

    fun pushTodos(settings: FirebaseSettings, store: TodoStore, session: FirebaseSession) {
        store.items.forEach { item ->
            val rev = item.updatedAt.toInstant().toEpochMilli()
            if (item.status == TodoRepository.STATUS_ARCHIVED) {
                val record = JSONObject()
                    .put("item", TodoRepository.itemJson(item.copy(text = "")))
                    .put("rev", rev)
                    .put("updated_by", session.uid)
                    .put("deleted", true)
                putDatabase(settings, "workspaces/${settings.workspaceId}/todos/${item.id}", record, session.idToken)
                return@forEach
            }
            val record = JSONObject()
                .put("item", TodoRepository.itemJson(item))
                .put("rev", rev)
                .put("updated_by", session.uid)
                .put("deleted", false)
            putDatabase(settings, "workspaces/${settings.workspaceId}/todos/${item.id}", record, session.idToken)
        }
        runCatching {
            putDatabaseBestEffort(
                settings,
                "workspaces/${settings.workspaceId}/todo_archive_months",
                org.json.JSONArray().apply { TodoRepository.archiveMonths(store).forEach { put(it) } }.toString(),
                session.idToken,
            )
        }
        val now = TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC))
        writeSyncHashBestEffort(settings, SYNC_FEATURE_TODOS, todoStoreHash(store), now, session)
        writeSyncHashBestEffort(settings, SYNC_FEATURE_TODO_ARCHIVE_MONTHS, todoArchiveMonthsHash(TodoRepository.archiveMonths(store)), now, session)
    }

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
        putDatabase(settings, "workspaces/${settings.workspaceId}/notes/$id", record, session.idToken)
        clearLocalSyncHash(settings, SYNC_FEATURE_NOTES)
    }

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
        putDatabase(settings, "workspaces/${settings.workspaceId}/notes/$id", record, session.idToken)
        clearLocalSyncHash(settings, SYNC_FEATURE_NOTES)
    }

    fun pullNotes(settings: FirebaseSettings, session: FirebaseSession): List<FirebaseRemoteNote> {
        syncHashes(settings, session)[SYNC_FEATURE_NOTES]?.let { remote ->
            if (shouldSkipFeature(settings, SYNC_FEATURE_NOTES, remote)) return emptyList()
        }
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/notes", session.idToken) ?: return emptyList()
        val notes = mutableListOf<FirebaseRemoteNote>()
        remote.keys().forEach { id ->
            val record = remote.optJSONObject(id) ?: return@forEach
            val path = record.optString("path", "")
            if (path.isBlank()) return@forEach
            notes += FirebaseRemoteNote(
                id = record.optString("id", id),
                path = NotesRepository.normalizePath(path).replace('\\', '/'),
                text = record.optString("text", ""),
                rev = record.optLong("rev", 0L),
                deleted = record.optBoolean("deleted", false),
            )
        }
        val sorted = notes.sortedBy { it.path.lowercase() }
        val hash = noteMetadataHash(sorted)
        markFeaturePulled(settings, SYNC_FEATURE_NOTES, hash)
        writeSyncHashBestEffort(settings, SYNC_FEATURE_NOTES, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        return sorted
    }

    fun pullTodos(settings: FirebaseSettings, local: TodoStore, session: FirebaseSession, forceFull: Boolean = false): TodoStore {
        val hashes = syncHashes(settings, session)
        val skipTodos = !forceFull && hashes[SYNC_FEATURE_TODOS]?.let { shouldSkipFeature(settings, SYNC_FEATURE_TODOS, it) } == true
        val skipMonths = !forceFull && hashes[SYNC_FEATURE_TODO_ARCHIVE_MONTHS]?.let { shouldSkipFeature(settings, SYNC_FEATURE_TODO_ARCHIVE_MONTHS, it) } == true
        val remoteItems = if (skipTodos) emptyList() else {
            pullRemoteTodos(settings, session)
                .filter { it.deleted || it.item.status != TodoRepository.STATUS_ARCHIVED }
        }
        val remoteArchiveMonths = if (skipMonths) emptyList() else pullTodoArchiveMonths(settings, session)
        val byId = if (forceFull && !skipTodos) {
            mutableMapOf()
        } else {
            TodoRepository.nonArchivedStore(local).items.associateBy { it.id }.toMutableMap()
        }
        val localArchivedById = local.items
            .filter { it.status == TodoRepository.STATUS_ARCHIVED }
            .associateBy { it.id }
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
        val merged = TodoRepository.preserveArchived(local, local.copy(items = sortedTodos(byId.values)))
        val normalized = TodoRepository.normalize(merged.copy(archiveMonths = merged.archiveMonths + remoteArchiveMonths))
        if (!skipTodos) {
            val hash = remoteTodoHash(remoteItems)
            markFeaturePulled(settings, SYNC_FEATURE_TODOS, hash)
            writeSyncHashBestEffort(settings, SYNC_FEATURE_TODOS, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        }
        if (!skipMonths) {
            val hash = todoArchiveMonthsHash(remoteArchiveMonths)
            markFeaturePulled(settings, SYNC_FEATURE_TODO_ARCHIVE_MONTHS, hash)
            writeSyncHashBestEffort(settings, SYNC_FEATURE_TODO_ARCHIVE_MONTHS, hash, TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)), session)
        }
        return normalized
    }

    fun pullRemoteTodoStore(settings: FirebaseSettings, session: FirebaseSession): TodoStore {
        return TodoStore(
            items = sortedTodos(
                pullRemoteTodos(settings, session)
                    .filterNot { it.deleted }
                    .map { it.item }
                    .filter { it.status != TodoRepository.STATUS_ARCHIVED },
            ),
        )
    }

    fun pullTodoArchiveMonth(settings: FirebaseSettings, month: String, session: FirebaseSession): List<TodoItem> {
        val feature = "$SYNC_FEATURE_TODO_ARCHIVE_MONTH_PREFIX$month"
        syncHashes(settings, session)[feature]?.let { remote ->
            if (shouldSkipFeature(settings, feature, remote)) return emptyList()
        }
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/todo_archives/$month", session.idToken)
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

    fun pullTodoArchiveMonths(settings: FirebaseSettings, session: FirebaseSession): List<String> {
        val response = request("GET", databaseUrl(settings, "workspaces/${settings.workspaceId}/todo_archive_months", session.idToken), null, null)
        if (response.isBlank() || response == "null") return emptyList()
        val values = org.json.JSONArray(response)
        return (0 until values.length()).mapNotNull { index ->
            values.optString(index).takeIf { it.isNotBlank() }
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

    private fun pullRemoteTodos(settings: FirebaseSettings, session: FirebaseSession): List<FirebaseRemoteTodo> {
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/todos", session.idToken)
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

    private fun clearLocalSyncHash(settings: FirebaseSettings, feature: String) {
        syncState.edit()
            .remove(syncHashKey(settings, feature))
            .remove(syncHashPulledAtKey(settings, feature))
            .apply()
    }

    private fun requestJson(method: String, url: String, body: String?, contentType: String): JSONObject {
        val response = request(method, url, body, contentType)
        return JSONObject(response)
    }

    private fun request(method: String, url: String, body: String?, contentType: String?): String {
        val connection = URL(url).openConnection() as HttpURLConnection
        connection.requestMethod = method
        connection.connectTimeout = 15_000
        connection.readTimeout = 20_000
        if (body != null) {
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", contentType ?: "application/json")
            connection.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
        }
        val stream = if (connection.responseCode in 200..299) connection.inputStream else connection.errorStream
        val response = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
        if (connection.responseCode !in 200..299) {
            throw IllegalStateException("Firebase request failed ${connection.responseCode}: $response")
        }
        return response
    }

    private fun databaseUrl(settings: FirebaseSettings, path: String, idToken: String): String {
        return settings.databaseUrl.trimEnd('/') + "/" + path.trim('/') + ".json?auth=${encode(idToken)}"
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
        private const val SYNC_FEATURE_TODO_ARCHIVE_MONTH_PREFIX = "todo_archive_month:"
        private const val SYNC_FEATURE_NOTES = "notes"
        private const val SYNC_FEATURE_SETTINGS = "settings"
        private const val SYNC_HASH_FULL_VALIDATION_MS = 24L * 60L * 60L * 1000L

        fun personalWorkspaceId(uid: String): String = "user_$uid"

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

        fun noteMetadataHash(notes: List<FirebaseRemoteNote>): String {
            val values = notes.sortedBy { it.id }.joinToString(separator = ",", prefix = "[", postfix = "]") { note ->
                val path = NotesRepository.normalizePath(note.path).replace('\\', '/')
                """{"id":${JSONObject.quote(note.id)},"path":${JSONObject.quote(path)},"rev":${note.rev},"deleted":${note.deleted}}"""
            }
            return sha256String(values)
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

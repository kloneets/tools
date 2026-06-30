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
import java.util.Locale
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

data class FirebaseRemoteAsset(
    val id: String,
    val path: String,
    val bytes: ByteArray,
    val rev: Long,
    val deleted: Boolean,
)

data class FirebaseSharedSettings(
    val values: JSONObject,
    val rev: Long,
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
        if (settings.workspaceId.isNotBlank() && settings.workspaceId != personalWorkspaceId) {
            return settings
        }
        val workspaceId = settings.workspaceId.ifBlank { personalWorkspaceId }
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
            if (item.status == TodoRepository.STATUS_ARCHIVED) return@forEach
            val rev = item.updatedAt.toInstant().toEpochMilli()
            val record = JSONObject()
                .put("item", TodoRepository.itemJson(item))
                .put("rev", rev)
                .put("updated_by", session.uid)
                .put("deleted", false)
            putDatabase(settings, "workspaces/${settings.workspaceId}/todos/${item.id}", record, session.idToken)
        }
        putDatabase(
            settings,
            "workspaces/${settings.workspaceId}/todo_archive_months",
            org.json.JSONArray().apply { TodoRepository.archiveMonths(store).forEach { put(it) } }.toString(),
            session.idToken,
        )
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
    }

    fun pullNotes(settings: FirebaseSettings, session: FirebaseSession): List<FirebaseRemoteNote> {
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
        return notes.sortedBy { it.path.lowercase() }
    }

    fun pullTodos(settings: FirebaseSettings, local: TodoStore, session: FirebaseSession): TodoStore {
        val remoteItems = pullRemoteTodos(settings, session)
            .filter { it.item.status != TodoRepository.STATUS_ARCHIVED }
        val remoteArchiveMonths = pullTodoArchiveMonths(settings, session)
        val byId = TodoRepository.nonArchivedStore(local).items.associateBy { it.id }.toMutableMap()
        remoteItems.forEach { record ->
            val item = record.item
            if (record.deleted) {
                byId.remove(item.id)
                return@forEach
            }
            val localItem = byId[item.id]
            if (localItem == null || !item.updatedAt.isBefore(localItem.updatedAt)) {
                byId[item.id] = item
            }
        }
        val merged = TodoRepository.preserveArchived(local, local.copy(items = sortedTodos(byId.values)))
        return TodoRepository.normalize(merged.copy(archiveMonths = merged.archiveMonths + remoteArchiveMonths))
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
        return records.sortedByDescending { it.archivedAt }
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
        return true
    }

    fun pullSharedSettings(settings: FirebaseSettings, session: FirebaseSession): FirebaseSharedSettings? {
        val record = getDatabase(settings, "workspaces/${settings.workspaceId}/settings/shared", session.idToken) ?: return null
        val values = record.optJSONObject("values") ?: return null
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

    fun pushAssets(settings: FirebaseSettings, assets: List<LocalAssetFile>, session: FirebaseSession): Int {
        var pushed = 0
        assets.forEach { asset ->
            if (asset.bytes.size > MAX_ASSET_BYTES) return@forEach
            val path = NotesRepository.normalizeAssetPath(asset.relativePath)
            if (path.isBlank()) return@forEach
            val id = assetId(path)
            val rev = maxOf(asset.lastModified, System.currentTimeMillis())
            val record = JSONObject()
                .put("id", id)
                .put("path", path)
                .put("bytes_base64", Base64.encodeToString(asset.bytes, Base64.NO_WRAP))
                .put("sha256", sha256(asset.bytes))
                .put("mime", mimeType(path))
                .put("rev", rev)
                .put("updated_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
                .put("updated_by", session.uid)
                .put("deleted", false)
            putDatabase(settings, "workspaces/${settings.workspaceId}/assets/$id", record, session.idToken)
            pushed++
        }
        return pushed
    }

    fun pushAssetDelete(settings: FirebaseSettings, path: String, session: FirebaseSession) {
        val normalized = NotesRepository.normalizeAssetPath(path)
        if (normalized.isBlank()) return
        val id = assetId(normalized)
        val rev = System.currentTimeMillis()
        val record = JSONObject()
            .put("id", id)
            .put("path", normalized)
            .put("rev", rev)
            .put("updated_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
            .put("updated_by", session.uid)
            .put("deleted", true)
        putDatabase(settings, "workspaces/${settings.workspaceId}/assets/$id", record, session.idToken)
    }


    fun pullAssets(settings: FirebaseSettings, session: FirebaseSession): List<FirebaseRemoteAsset> {
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/assets", session.idToken) ?: return emptyList()
        val assets = mutableListOf<FirebaseRemoteAsset>()
        remote.keys().forEach { id ->
            val record = remote.optJSONObject(id) ?: return@forEach
            val path = NotesRepository.normalizeAssetPath(record.optString("path", ""))
            if (path.isBlank()) return@forEach
            val deleted = record.optBoolean("deleted", false)
            val bytes = if (deleted) ByteArray(0) else runCatching {
                Base64.decode(record.optString("bytes_base64", ""), Base64.NO_WRAP)
            }.getOrDefault(ByteArray(0))
            if (!deleted && (bytes.isEmpty() || bytes.size > MAX_ASSET_BYTES)) return@forEach
            if (!deleted && record.optString("sha256", "").isNotBlank() && record.optString("sha256") != sha256(bytes)) return@forEach
            assets += FirebaseRemoteAsset(
                id = record.optString("id", id),
                path = path,
                bytes = bytes,
                rev = record.optLong("rev", 0L),
                deleted = deleted,
            )
        }
        return assets.sortedBy { it.path.lowercase() }
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
        requestJson("PUT", databaseUrl(settings, path, idToken), body.toString(), "application/json")
    }

    private fun putDatabase(settings: FirebaseSettings, path: String, body: String, idToken: String) {
        requestJson("PUT", databaseUrl(settings, path, idToken), body, "application/json")
    }

    private fun getDatabase(settings: FirebaseSettings, path: String, idToken: String): JSONObject? {
        val response = request("GET", databaseUrl(settings, path, idToken), null, null)
        if (response.isBlank() || response == "null") return null
        return JSONObject(response)
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

    private fun noteId(path: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(path.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }

    private fun assetId(path: String): String = noteId(NotesRepository.normalizeAssetPath(path))

    private fun sha256(bytes: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(bytes)
        return digest.joinToString("") { "%02x".format(it) }
    }

    private fun mimeType(path: String): String {
        return when (path.substringAfterLast('.', "").lowercase(Locale.US)) {
            "png" -> "image/png"
            "jpg", "jpeg" -> "image/jpeg"
            "gif" -> "image/gif"
            "webp" -> "image/webp"
            "svg" -> "image/svg+xml"
            "pdf" -> "application/pdf"
            "txt" -> "text/plain"
            else -> "application/octet-stream"
        }
    }

    companion object {
        const val TOKEN_FILE = "firebase_token.json"
        const val MAX_ASSET_BYTES = 1 * 1024 * 1024
        private const val SYNC_STATE_PREFERENCES = "firebase_sync_state"

        fun personalWorkspaceId(uid: String): String = "user_$uid"

        fun googleSignInPostBody(googleIdToken: String): String {
            return "id_token=${encodeQueryComponent(googleIdToken)}&providerId=${encodeQueryComponent("google.com")}"
        }

        fun sharedSettingsHash(values: JSONObject): String {
            val digest = MessageDigest.getInstance("SHA-256").digest(canonicalJson(values).toByteArray(Charsets.UTF_8))
            return digest.joinToString("") { "%02x".format(it) }
        }

        private fun canonicalJson(value: Any?): String {
            return when (value) {
                null, JSONObject.NULL -> "null"
                is JSONObject -> {
                    val keys = value.keys().asSequence().toList().sorted()
                    keys.joinToString(prefix = "{", postfix = "}") { key ->
                        JSONObject.quote(key) + ":" + canonicalJson(value.opt(key))
                    }
                }
                is org.json.JSONArray -> {
                    (0 until value.length()).joinToString(prefix = "[", postfix = "]") { index ->
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
    }

    private fun sharedSettingsHashKey(settings: FirebaseSettings): String {
        return "shared_settings_hash:${settings.workspaceId.ifBlank { "default" }}"
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

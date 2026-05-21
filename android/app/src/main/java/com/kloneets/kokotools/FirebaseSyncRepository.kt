package com.kloneets.kokotools

import android.content.Context
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URLEncoder
import java.net.URL
import java.time.OffsetDateTime
import java.time.ZoneOffset

data class FirebaseSession(
    val uid: String,
    val email: String,
    val idToken: String,
    val refreshToken: String,
)

class FirebaseSyncRepository(private val context: Context) {
    private val tokenFile: File
        get() = File(context.filesDir, TOKEN_FILE)

    fun configured(settings: FirebaseSettings): Boolean {
        return settings.enabled &&
            settings.realtime &&
            settings.apiKey.isNotBlank() &&
            settings.databaseUrl.isNotBlank() &&
            settings.workspaceId.isNotBlank()
    }

    fun currentSession(settings: FirebaseSettings): FirebaseSession? {
        val saved = loadToken() ?: return null
        return refresh(settings, saved.refreshToken).getOrNull()
    }

    fun login(settings: FirebaseSettings, email: String, password: String): FirebaseSession {
        val body = JSONObject()
            .put("email", email)
            .put("password", password)
            .put("returnSecureToken", true)
        val response = postJson(
            "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${encode(settings.apiKey)}",
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
            val record = JSONObject()
                .put("item", TodoRepository.itemJson(item))
                .put("rev", rev)
                .put("updated_by", session.uid)
                .put("deleted", false)
            putDatabase(settings, "workspaces/${settings.workspaceId}/todos/${item.id}", record, session.idToken)
        }
        val event = JSONObject()
            .put("device_id", "android")
            .put("created_at", TodoRepository.formatTime(OffsetDateTime.now(ZoneOffset.UTC)))
            .put("kind", "todos_push")
        putDatabase(
            settings,
            "workspaces/${settings.workspaceId}/events/android-${System.currentTimeMillis()}",
            event,
            session.idToken,
        )
    }

    fun pullTodos(settings: FirebaseSettings, local: TodoStore, session: FirebaseSession): TodoStore {
        val remote = getDatabase(settings, "workspaces/${settings.workspaceId}/todos", session.idToken)
        val byId = local.items.associateBy { it.id }.toMutableMap()
        if (remote != null) {
            remote.keys().forEach { id ->
                val record = remote.optJSONObject(id) ?: return@forEach
                if (record.optBoolean("deleted", false)) {
                    byId.remove(id)
                    return@forEach
                }
                val itemJson = record.optJSONObject("item") ?: return@forEach
                val item = TodoRepository.parseItem(itemJson)
                val localItem = byId[item.id]
                if (localItem == null || !item.updatedAt.isBefore(localItem.updatedAt)) {
                    byId[item.id] = item
                }
            }
        }
        return local.copy(items = byId.values.sortedWith(compareBy<TodoItem> { it.status }.thenBy { it.order }.thenBy { it.createdAt }))
    }

    private fun loadToken(): FirebaseSession? {
        if (!tokenFile.isFile) return null
        return runCatching {
            val json = JSONObject(tokenFile.readText())
            FirebaseSession(
                uid = json.optString("uid"),
                email = json.optString("email"),
                idToken = json.optString("id_token"),
                refreshToken = json.optString("refresh_token"),
            )
        }.getOrNull()
    }

    private fun saveToken(session: FirebaseSession) {
        context.filesDir.mkdirs()
        tokenFile.writeText(
            JSONObject()
                .put("uid", session.uid)
                .put("email", session.email)
                .put("id_token", session.idToken)
                .put("refresh_token", session.refreshToken)
                .toString(2),
        )
    }

    private fun postJson(url: String, body: JSONObject): JSONObject {
        return requestJson("POST", url, body.toString(), "application/json")
    }

    private fun putDatabase(settings: FirebaseSettings, path: String, body: JSONObject, idToken: String) {
        requestJson("PUT", databaseUrl(settings, path, idToken), body.toString(), "application/json")
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

    companion object {
        const val TOKEN_FILE = "firebase_token.json"
    }
}

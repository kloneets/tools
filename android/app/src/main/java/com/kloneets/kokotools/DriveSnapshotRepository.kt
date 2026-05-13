package com.kloneets.kokotools

import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLEncoder
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter

class DriveSnapshotRepository {
    data class DriveEntry(
        val id: String,
        val name: String,
        val mimeType: String,
        val createdTime: String = "",
        val modifiedTime: String = "",
    )

    data class DriveFileListPage(
        val files: List<DriveEntry>,
        val nextPageToken: String = "",
    )

    class SnapshotsFolderNotFoundException : IllegalStateException("Drive snapshots folder not found")
    class DriveAuthorizationException(message: String) : IOException(message)

    fun snapshotsQuery(parentFolderId: String): String {
        val escaped = escapeQueryValue(parentFolderId)
        return "'$escaped' in parents and name = 'snapshots' and mimeType = '$DRIVE_FOLDER_MIME' and trashed = false"
    }

    fun snapshotChildrenQuery(snapshotFolderId: String): String {
        val escaped = escapeQueryValue(snapshotFolderId)
        return "'$escaped' in parents and trashed = false"
    }

    fun snapshotFoldersQuery(snapshotsRootId: String): String {
        val escaped = escapeQueryValue(snapshotsRootId)
        return "'$escaped' in parents and mimeType = '$DRIVE_FOLDER_MIME' and trashed = false"
    }

    fun namedFolderQuery(parentFolderId: String, name: String): String {
        val escapedParent = escapeQueryValue(parentFolderId)
        val escapedName = escapeQueryValue(name)
        return "'$escapedParent' in parents and name = '$escapedName' and mimeType = '$DRIVE_FOLDER_MIME' and trashed = false"
    }

    fun childrenQuery(parentFolderId: String): String {
        val escaped = escapeQueryValue(parentFolderId)
        return "'$escaped' in parents and trashed = false"
    }

    fun urlEncode(value: String): String {
        return URLEncoder.encode(value, Charsets.UTF_8.name())
    }

    fun multipartBody(metadata: JSONObject, contentType: String, bytes: ByteArray, boundary: String): ByteArray {
        val prefix = buildString {
            append("--").append(boundary).append("\r\n")
            append("Content-Type: application/json; charset=UTF-8\r\n\r\n")
            append(metadata.toString()).append("\r\n")
            append("--").append(boundary).append("\r\n")
            append("Content-Type: ").append(contentType).append("\r\n\r\n")
        }.toByteArray(Charsets.UTF_8)
        val suffix = "\r\n--$boundary--\r\n".toByteArray(Charsets.UTF_8)
        return prefix + bytes + suffix
    }

    fun parseSnapshotMetadata(raw: String): List<DriveSnapshotMeta> {
        return parseDriveFileListPage(raw).files
            .filter { it.mimeType == DRIVE_FOLDER_MIME }
            .map {
                DriveSnapshotMeta(
                    id = it.id,
                    name = it.name,
                    createdAt = snapshotCreatedAt(it),
                )
            }
            .sortedWith(snapshotMetaComparator())
    }

    fun parseDriveFileListPage(raw: String): DriveFileListPage {
        val json = JSONObject(raw)
        val files = json.optJSONArray("files") ?: JSONArray()
        return DriveFileListPage(
            files = (0 until files.length()).mapNotNull { index ->
                files.optJSONObject(index)?.let {
                    DriveEntry(
                        id = it.optString("id"),
                        name = it.optString("name"),
                        mimeType = it.optString("mimeType"),
                        createdTime = it.optString("createdTime"),
                        modifiedTime = it.optString("modifiedTime"),
                    )
                }
            },
            nextPageToken = json.optString("nextPageToken"),
        )
    }

    fun listFilesUrl(query: String, pageToken: String = ""): String {
        val fields = "nextPageToken,files(id,name,mimeType,createdTime,modifiedTime)"
        return buildString {
            append(DRIVE_FILES_URL)
            append("?q=").append(urlEncode(query))
            append("&fields=").append(urlEncode(fields))
            append("&pageSize=1000")
            if (pageToken.isNotBlank()) {
                append("&pageToken=").append(urlEncode(pageToken))
            }
        }
    }

    fun uploadSnapshot(
        folderId: String,
        accessToken: String,
        settingsData: ByteArray,
        notesRoot: File,
        retain: Int = 5,
    ): DriveSnapshotMeta {
        require(folderId.isNotBlank()) { "Drive folder ID is required" }
        val snapshotsRootId = ensureFolder(accessToken, folderId, SNAPSHOTS_DIR)
        val snapshot = createFolder(accessToken, snapshotsRootId, timestampName())

        uploadFile(accessToken, snapshot.id, "settings.json", "application/json", settingsData)
        val notesFolderId = createFolder(accessToken, snapshot.id, NOTES_DIR).id
        uploadNotesTree(accessToken, notesFolderId, notesRoot)
        pruneOldSnapshots(accessToken, snapshotsRootId, retain)

        return DriveSnapshotMeta(
            id = snapshot.id,
            name = snapshot.name,
            createdAt = snapshot.modifiedTime.ifBlank { snapshot.createdTime },
        )
    }

    fun listSnapshots(folderId: String, accessToken: String): List<DriveSnapshotMeta> {
        require(folderId.isNotBlank()) { "Drive folder ID is required" }
        val snapshotsRoot = findFolder(accessToken, folderId, SNAPSHOTS_DIR)
            ?: throw SnapshotsFolderNotFoundException()
        return listByQuery(accessToken, snapshotFoldersQuery(snapshotsRoot.id))
            .map {
                DriveSnapshotMeta(
                    id = it.id,
                    name = it.name,
                    createdAt = snapshotCreatedAt(it),
                )
            }
            .sortedWith(snapshotMetaComparator())
    }

    fun restoreSnapshot(
        snapshotId: String,
        accessToken: String,
        notesRepository: NotesRepository,
    ): ByteArray {
        require(snapshotId.isNotBlank()) { "Snapshot ID is required" }
        val settingsEntry = findFile(accessToken, snapshotId, "settings.json")
            ?: throw IllegalStateException("Snapshot settings.json not found")
        val settingsData = downloadFile(accessToken, settingsEntry.id)

        val notesEntry = findFolder(accessToken, snapshotId, NOTES_DIR)
            ?: throw IllegalStateException("Snapshot notes folder not found")
        notesRepository.clearAll()
        restoreNotesTree(accessToken, notesEntry.id, "", notesRepository)
        return settingsData
    }

    private fun uploadNotesTree(accessToken: String, notesFolderId: String, notesRoot: File) {
        if (!notesRoot.isDirectory) return

        val folderIds = mutableMapOf("" to notesFolderId)
        notesRoot.walkTopDown()
            .filter { it != notesRoot }
            .sortedBy { it.relativeTo(notesRoot).invariantSeparatorsPath }
            .forEach { file ->
                val rel = file.relativeTo(notesRoot).invariantSeparatorsPath
                if (file.isDirectory) {
                    folderIds[rel] = ensureFolderPath(accessToken, notesFolderId, rel, folderIds)
                } else if (file.isFile) {
                    val parentRel = rel.substringBeforeLast("/", "")
                    val parentId = ensureFolderPath(accessToken, notesFolderId, parentRel, folderIds)
                    uploadFile(accessToken, parentId, file.name, contentTypeFor(file.name), file.readBytes())
                }
            }
    }

    private fun restoreNotesTree(
        accessToken: String,
        driveFolderId: String,
        relativeDir: String,
        notesRepository: NotesRepository,
    ) {
        for (child in listChildren(accessToken, driveFolderId)) {
            val childRel = listOf(relativeDir, child.name).filter { it.isNotBlank() }.joinToString("/")
            if (child.mimeType == DRIVE_FOLDER_MIME) {
                notesRepository.resolveLocalPath(childRel).mkdirs()
                restoreNotesTree(accessToken, child.id, childRel, notesRepository)
            } else {
                val localFile = notesRepository.resolveLocalPath(childRel)
                localFile.parentFile?.mkdirs()
                localFile.writeBytes(downloadFile(accessToken, child.id))
            }
        }
    }

    private fun ensureFolderPath(
        accessToken: String,
        rootFolderId: String,
        relativePath: String,
        folderIds: MutableMap<String, String>,
    ): String {
        if (relativePath.isBlank()) return rootFolderId
        folderIds[relativePath]?.let { return it }

        var parentId = rootFolderId
        var currentPath = ""
        for (part in relativePath.split('/').filter { it.isNotBlank() }) {
            currentPath = listOf(currentPath, part).filter { it.isNotBlank() }.joinToString("/")
            parentId = folderIds[currentPath] ?: ensureFolder(accessToken, parentId, part).also {
                folderIds[currentPath] = it
            }
        }
        return parentId
    }

    private fun ensureFolder(accessToken: String, parentFolderId: String, name: String): String {
        return findFolder(accessToken, parentFolderId, name)?.id ?: createFolder(accessToken, parentFolderId, name).id
    }

    private fun findFolder(accessToken: String, parentFolderId: String, name: String): DriveEntry? {
        return listByQuery(accessToken, namedFolderQuery(parentFolderId, name)).firstOrNull()
    }

    private fun findFile(accessToken: String, parentFolderId: String, name: String): DriveEntry? {
        val query = "'${escapeQueryValue(parentFolderId)}' in parents and name = '${escapeQueryValue(name)}' and trashed = false"
        return listByQuery(accessToken, query).firstOrNull { it.mimeType != DRIVE_FOLDER_MIME }
    }

    private fun listChildren(accessToken: String, parentFolderId: String): List<DriveEntry> {
        return listByQuery(accessToken, childrenQuery(parentFolderId))
    }

    private fun listByQuery(accessToken: String, query: String): List<DriveEntry> {
        val entries = mutableListOf<DriveEntry>()
        var pageToken = ""
        do {
            val body = request(accessToken, "GET", listFilesUrl(query, pageToken))
            val page = parseDriveFileListPage(body.toString(Charsets.UTF_8))
            entries += page.files
            pageToken = page.nextPageToken
        } while (pageToken.isNotBlank())
        return entries
    }

    private fun createFolder(accessToken: String, parentFolderId: String, name: String): DriveEntry {
        val metadata = JSONObject()
            .put("name", name)
            .put("mimeType", DRIVE_FOLDER_MIME)
            .put("parents", JSONArray().put(parentFolderId))
        val body = request(
            accessToken = accessToken,
            method = "POST",
            url = "$DRIVE_FILES_URL?fields=${urlEncode("id,name,mimeType,createdTime,modifiedTime")}",
            contentType = "application/json; charset=UTF-8",
            body = metadata.toString().toByteArray(Charsets.UTF_8),
        )
        val json = JSONObject(body.toString(Charsets.UTF_8))
        return DriveEntry(
            id = json.getString("id"),
            name = json.optString("name", name),
            mimeType = json.optString("mimeType", DRIVE_FOLDER_MIME),
            createdTime = json.optString("createdTime"),
            modifiedTime = json.optString("modifiedTime"),
        )
    }

    private fun uploadFile(accessToken: String, parentFolderId: String, name: String, contentType: String, bytes: ByteArray): DriveEntry {
        val boundary = "koko-tools-${System.currentTimeMillis()}"
        val metadata = JSONObject()
            .put("name", name)
            .put("parents", JSONArray().put(parentFolderId))
        val body = multipartBody(metadata, contentType, bytes, boundary)
        val response = request(
            accessToken = accessToken,
            method = "POST",
            url = "$DRIVE_UPLOAD_URL?uploadType=multipart&fields=${urlEncode("id,name,mimeType,createdTime,modifiedTime")}",
            contentType = "multipart/related; boundary=$boundary",
            body = body,
        )
        val json = JSONObject(response.toString(Charsets.UTF_8))
        return DriveEntry(
            id = json.getString("id"),
            name = json.optString("name", name),
            mimeType = json.optString("mimeType"),
            createdTime = json.optString("createdTime"),
            modifiedTime = json.optString("modifiedTime"),
        )
    }

    private fun downloadFile(accessToken: String, fileId: String): ByteArray {
        return request(accessToken, "GET", "$DRIVE_FILES_URL/${urlEncode(fileId)}?alt=media")
    }

    private fun pruneOldSnapshots(accessToken: String, snapshotsRootId: String, retain: Int) {
        if (retain <= 0) return
        val snapshots = listByQuery(accessToken, snapshotFoldersQuery(snapshotsRootId))
            .sortedByDescending { it.modifiedTime.ifBlank { it.createdTime } }
        snapshots.drop(retain).forEach {
            request(accessToken, "DELETE", "$DRIVE_FILES_URL/${urlEncode(it.id)}")
        }
    }

    private fun request(
        accessToken: String,
        method: String,
        url: String,
        contentType: String? = null,
        body: ByteArray? = null,
    ): ByteArray {
        val connection = (URL(url).openConnection() as HttpURLConnection).apply {
            requestMethod = method
            connectTimeout = REQUEST_TIMEOUT_MS
            readTimeout = REQUEST_TIMEOUT_MS
            setRequestProperty("Authorization", "Bearer $accessToken")
            if (contentType != null) {
                setRequestProperty("Content-Type", contentType)
            }
            if (body != null) {
                doOutput = true
                outputStream.use { it.write(body) }
            }
        }

        val responseCode = connection.responseCode
        val stream = if (responseCode in 200..299) connection.inputStream else connection.errorStream
        val response = stream?.use { input ->
            ByteArrayOutputStream().use { output ->
                input.copyTo(output)
                output.toByteArray()
            }
        } ?: ByteArray(0)
        connection.disconnect()
        if (responseCode !in 200..299) {
            val message = "Drive API $method $url failed with HTTP $responseCode: ${response.toString(Charsets.UTF_8)}"
            if (responseCode == HttpURLConnection.HTTP_UNAUTHORIZED || responseCode == HttpURLConnection.HTTP_FORBIDDEN) {
                throw DriveAuthorizationException(message)
            }
            throw IOException(message)
        }
        return response
    }

    private fun snapshotCreatedAt(entry: DriveEntry): String {
        return entry.modifiedTime.ifBlank { entry.createdTime }
    }

    private fun snapshotMetaComparator(): Comparator<DriveSnapshotMeta> {
        return compareByDescending<DriveSnapshotMeta> { it.createdAt }
            .thenByDescending { it.name }
    }

    private fun timestampName(): String {
        return OffsetDateTime.now(ZoneOffset.UTC).format(DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH-mm-ssXXX"))
    }

    private fun contentTypeFor(name: String): String {
        return if (name.endsWith(".md", ignoreCase = true)) "text/markdown; charset=UTF-8" else "application/octet-stream"
    }

    private fun escapeQueryValue(value: String): String {
        return value.replace("\\", "\\\\").replace("'", "\\'")
    }

    companion object {
        const val DRIVE_SCOPE = "https://www.googleapis.com/auth/drive"
        const val DRIVE_FOLDER_MIME = "application/vnd.google-apps.folder"
        private const val DRIVE_FILES_URL = "https://www.googleapis.com/drive/v3/files"
        private const val DRIVE_UPLOAD_URL = "https://www.googleapis.com/upload/drive/v3/files"
        private const val REQUEST_TIMEOUT_MS = 15_000
        private const val SNAPSHOTS_DIR = "snapshots"
        private const val NOTES_DIR = "notes"
    }
}

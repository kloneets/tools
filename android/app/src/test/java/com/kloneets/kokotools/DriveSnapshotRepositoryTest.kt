package com.kloneets.kokotools

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DriveSnapshotRepositoryTest {
    private val repository = DriveSnapshotRepository()

    @Test
    fun buildsSnapshotQueries() {
        assertEquals(
            "'folder' in parents and name = 'snapshots' and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
            repository.snapshotsQuery("folder"),
        )
        assertEquals(
            "'snapshot' in parents and trashed = false",
            repository.snapshotChildrenQuery("snapshot"),
        )
        assertEquals(
            "'snapshots-root' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
            repository.snapshotFoldersQuery("snapshots-root"),
        )
        assertEquals(
            "'folder\\'id' in parents and name = 'snapshots' and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
            repository.snapshotsQuery("folder'id"),
        )
    }

    @Test
    fun buildsNamedFolderAndChildrenQueries() {
        assertEquals(
            "'folder' in parents and name = 'notes' and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
            repository.namedFolderQuery("folder", "notes"),
        )
        assertEquals(
            "'folder' in parents and trashed = false",
            repository.childrenQuery("folder"),
        )
    }

    @Test
    fun parsesSnapshotMetadataNewestFirst() {
        val snapshots = repository.parseSnapshotMetadata(
            """
            {
              "files": [
                {"id": "old", "name": "old", "mimeType": "application/vnd.google-apps.folder", "modifiedTime": "2026-01-01T00:00:00Z"},
                {"id": "ignored-file", "name": "settings.json", "mimeType": "application/json", "modifiedTime": "2026-06-01T00:00:00Z"},
                {"id": "new", "name": "new", "mimeType": "application/vnd.google-apps.folder", "modifiedTime": "2026-05-13T00:00:00Z"}
              ]
            }
            """.trimIndent(),
        )

        assertEquals(2, snapshots.size)
        assertEquals("new", snapshots.first().id)
        assertEquals("old", snapshots.last().id)
    }

    @Test
    fun parsesSnapshotMetadataWithCreatedTimeFallback() {
        val snapshots = repository.parseSnapshotMetadata(
            """
            {
              "files": [
                {"id": "created", "name": "created", "mimeType": "application/vnd.google-apps.folder", "createdTime": "2026-05-13T00:00:00Z"},
                {"id": "named", "name": "zz-name", "mimeType": "application/vnd.google-apps.folder"},
                {"id": "modified", "name": "modified", "mimeType": "application/vnd.google-apps.folder", "createdTime": "2026-01-01T00:00:00Z", "modifiedTime": "2026-06-01T00:00:00Z"}
              ]
            }
            """.trimIndent(),
        )

        assertEquals(listOf("modified", "created", "named"), snapshots.map { it.id })
        assertEquals("2026-05-13T00:00:00Z", snapshots[1].createdAt)
    }

    @Test
    fun parsesPaginatedDriveFileListPage() {
        val page = repository.parseDriveFileListPage(
            """
            {
              "nextPageToken": "next-token",
              "files": [
                {"id": "one", "name": "first", "mimeType": "application/vnd.google-apps.folder", "createdTime": "2026-01-01T00:00:00Z"}
              ]
            }
            """.trimIndent(),
        )

        assertEquals("next-token", page.nextPageToken)
        assertEquals("one", page.files.single().id)
    }

    @Test
    fun buildsPagedDriveListUrl() {
        val url = repository.listFilesUrl(repository.childrenQuery("folder"), "next page")

        assertTrue(url.contains("pageSize=1000"))
        assertTrue(url.contains("nextPageToken%2Cfiles"))
        assertTrue(url.contains("pageToken=next+page"))
    }

    @Test
    fun buildsMultipartUploadBody() {
        val body = repository.multipartBody(
            metadata = JSONObject().put("name", "settings.json"),
            contentType = "application/json",
            bytes = "{}".toByteArray(),
            boundary = "boundary",
        ).toString(Charsets.UTF_8)

        assertTrue(body.contains("Content-Type: application/json; charset=UTF-8"))
        assertTrue(body.contains("\"name\":\"settings.json\""))
        assertTrue(body.contains("--boundary--"))
    }
}

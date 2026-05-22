package com.kloneets.kokotools

import android.content.Context
import java.io.File

class NotesRepository(private val context: Context) {
    private val notesRoot: File
        get() = File(context.filesDir, NOTES_DIR)

    fun listNotes(): List<NoteFile> {
        notesRoot.mkdirs()
        return notesRoot
            .walkTopDown()
            .filter { it.isFile && it.extension.equals("md", ignoreCase = true) }
            .map { NoteFile(relativePath = it.relativeTo(notesRoot).invariantSeparatorsPath) }
            .sortedBy { it.relativePath.lowercase() }
            .toList()
    }

    fun read(relativePath: String): String {
        val file = resolveNote(relativePath)
        return file.takeIf { it.isFile }?.readText() ?: ""
    }

    fun save(relativePath: String, contents: String): NoteFile {
        val normalized = normalizePath(relativePath)
        val file = resolveNote(normalized)
        file.parentFile?.mkdirs()
        file.writeText(contents)
        return NoteFile(normalized)
    }

    fun delete(relativePath: String): Boolean {
        return resolveNote(relativePath).delete()
    }

    fun listAssets(): List<LocalAssetFile> {
        notesRoot.mkdirs()
        return notesRoot
            .walkTopDown()
            .filter { it.isFile }
            .map { it.relativeTo(notesRoot).invariantSeparatorsPath }
            .filter { isManagedAssetPath(it) }
            .sortedBy { it.lowercase() }
            .mapNotNull { rel ->
                val file = resolveLocalPath(rel)
                if (!file.isFile) null else LocalAssetFile(rel, file.readBytes(), file.lastModified())
            }
            .toList()
    }

    fun writeAsset(relativePath: String, bytes: ByteArray) {
        val file = resolveLocalPath(normalizeAssetPath(relativePath))
        file.parentFile?.mkdirs()
        file.writeBytes(bytes)
    }

    fun deleteAsset(relativePath: String): Boolean {
        return resolveLocalPath(normalizeAssetPath(relativePath)).delete()
    }

    fun notesPath(): File = notesRoot

    fun clearAll() {
        if (notesRoot.exists()) {
            notesRoot.deleteRecursively()
        }
        notesRoot.mkdirs()
    }

    fun resolveLocalPath(relativePath: String): File {
        val normalized = relativePath
            .trim()
            .replace('\\', '/')
            .split('/')
            .filter { it.isNotBlank() && it != "." && it != ".." }
            .joinToString("/")
        val file = File(notesRoot, normalized).canonicalFile
        val root = notesRoot.canonicalFile
        require(file.path == root.path || file.path.startsWith(root.path + File.separator)) {
            "Path escapes notes root"
        }
        return file
    }

    fun resolveNote(relativePath: String): File {
        val normalized = normalizePath(relativePath)
        val file = File(notesRoot, normalized).canonicalFile
        val root = notesRoot.canonicalFile
        require(file.path == root.path || file.path.startsWith(root.path + File.separator)) {
            "Note path escapes notes root"
        }
        return file
    }

    companion object {
        const val NOTES_DIR = "notes"

        fun normalizePath(input: String): String {
            val clean = input
                .trim()
                .replace('\\', '/')
                .split('/')
                .filter { it.isNotBlank() && it != "." && it != ".." }
                .joinToString("/")
            val withName = clean.ifBlank { "untitled.md" }
            return if (withName.endsWith(".md", ignoreCase = true)) withName else "$withName.md"
        }

        fun normalizeAssetPath(input: String): String {
            return input
                .trim()
                .replace('\\', '/')
                .split('/')
                .filter { it.isNotBlank() && it != "." && it != ".." }
                .joinToString("/")
        }

        fun isManagedAssetPath(relativePath: String): Boolean {
            return normalizeAssetPath(relativePath)
                .split('/')
                .any { it == "assets" || it.endsWith(".assets") }
        }

        fun managedAssetPathForNote(notePath: String, fileName: String): String {
            val cleanName = normalizeAssetPath(fileName).substringAfterLast('/').ifBlank { "asset" }
            val note = normalizePath(notePath).takeIf { notePath.isNotBlank() }
            if (note == null) return "assets/$cleanName"
            val folder = note.removeSuffix(".md") + ".assets"
            return normalizeAssetPath("$folder/$cleanName")
        }
    }
}

data class LocalAssetFile(
    val relativePath: String,
    val bytes: ByteArray,
    val lastModified: Long,
)

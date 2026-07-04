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

    fun notesPath(): File = notesRoot

    fun cleanupManagedAssetDirs() {
        if (!notesRoot.exists()) return
        notesRoot.walkTopDown()
            .filter { it.isDirectory && it != notesRoot && isManagedAssetDirName(it.name) }
            .toList()
            .forEach { it.deleteRecursively() }
    }

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

        fun isManagedAssetDirName(name: String): Boolean = name == "assets" || name.endsWith(".assets")
    }
}

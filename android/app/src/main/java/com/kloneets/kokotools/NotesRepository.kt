package com.kloneets.kokotools

import android.content.Context
import java.io.File
import java.io.FileOutputStream
import java.nio.file.AtomicMoveNotSupportedException
import java.nio.file.Files
import java.nio.file.StandardCopyOption

data class NoteVersion(
    val relativePath: String,
    val versionId: String,
    val createdAtMillis: Long,
    val sizeBytes: Long,
)

class NotesRepository internal constructor(
    private val filesDir: File,
    private val clock: () -> Long = System::currentTimeMillis,
    private val maxVersionsPerNote: Int = DEFAULT_MAX_VERSIONS_PER_NOTE,
    private val maxVersionsTotal: Int = DEFAULT_MAX_VERSIONS_TOTAL,
    private val deleteFile: (File) -> Boolean = File::delete,
) {
    constructor(context: Context) : this(
        filesDir = context.filesDir,
        clock = System::currentTimeMillis,
        maxVersionsPerNote = DEFAULT_MAX_VERSIONS_PER_NOTE,
        maxVersionsTotal = DEFAULT_MAX_VERSIONS_TOTAL,
        deleteFile = File::delete,
    )

    init {
        require(maxVersionsPerNote > 0) { "Per-note backup retention must be positive" }
        require(maxVersionsTotal > 0) { "Total backup retention must be positive" }
    }

    private val notesRoot: File
        get() = File(filesDir, NOTES_DIR)
    private val backupsRoot: File
        get() = File(filesDir, NOTE_BACKUPS_DIR)

    fun listNotes(): List<NoteFile> {
        notesRoot.mkdirs()
        return notesRoot
            .walkTopDown()
            .filter { it.isFile && it.extension.equals("md", ignoreCase = true) }
            .map { NoteFile(relativePath = it.relativeTo(notesRoot).invariantSeparatorsPath) }
            .sortedBy { it.relativePath.lowercase() }
            .toList()
    }

    fun listFolders(): List<String> {
        notesRoot.mkdirs()
        val directoryPaths = notesRoot
            .walkTopDown()
            .filter { it.isDirectory && it != notesRoot }
            .map { it.relativeTo(notesRoot).invariantSeparatorsPath }
            .toList()
        return collectFolders(listNotes().map { it.relativePath }, directoryPaths)
    }

    fun read(relativePath: String): String {
        val file = resolveNote(relativePath)
        return file.takeIf { it.isFile }?.readText() ?: ""
    }

    @Synchronized
    fun save(relativePath: String, contents: String): NoteFile {
        val normalized = normalizePath(relativePath)
        val file = resolveNote(normalized)
        val bytes = contents.toByteArray(Charsets.UTF_8)
        if (file.isFile) {
            val previous = file.readBytes()
            if (previous.contentEquals(bytes)) return NoteFile(normalized)
            createBackup(normalized, previous)
        }
        writeDurably(file, bytes)
        return NoteFile(normalized)
    }

    @Synchronized
    fun delete(relativePath: String): Boolean {
        val normalized = normalizePath(relativePath)
        val file = resolveNote(normalized)
        if (!file.exists()) return false
        if (file.isFile) createBackup(normalized, file.readBytes())
        check(deleteFile(file)) { "Failed to delete note $normalized" }
        return true
    }

    fun notesPath(): File = notesRoot

    @Synchronized
    fun cleanupManagedAssetDirs() {
        if (!notesRoot.exists()) return
        val managedDirectories = notesRoot.walkTopDown()
            .filter { it.isDirectory && it != notesRoot && isManagedAssetDirName(it.name) }
            .toList()
        managedDirectories
            .asSequence()
            .flatMap { directory -> directory.walkTopDown().filter { it.isFile && it.extension.equals("md", true) } }
            .distinctBy { it.canonicalPath }
            .forEach { file ->
                createBackup(file.relativeTo(notesRoot).invariantSeparatorsPath, file.readBytes())
            }
        managedDirectories.forEach { it.deleteRecursively() }
    }

    @Synchronized
    fun clearAll() {
        if (notesRoot.exists()) {
            listNotes().forEach { note ->
                val file = resolveNote(note.relativePath)
                createBackup(note.relativePath, file.readBytes())
            }
            check(notesRoot.deleteRecursively()) { "Failed to clear notes directory" }
        }
        check(notesRoot.mkdirs() || notesRoot.isDirectory) { "Failed to create notes directory" }
    }

    @Synchronized
    fun listVersions(relativePath: String): List<NoteVersion> {
        val normalized = normalizePath(relativePath)
        val directory = resolveBackupDirectory(normalized)
        if (!directory.isDirectory) return emptyList()
        return directory.listFiles().orEmpty()
            .asSequence()
            .filter { it.isFile && VERSION_FILE_PATTERN.matches(it.name) }
            .map { file ->
                NoteVersion(
                    relativePath = normalized,
                    versionId = file.name,
                    createdAtMillis = versionTimestamp(file.name),
                    sizeBytes = file.length(),
                )
            }
            .sortedWith(
                compareByDescending<NoteVersion> { it.createdAtMillis }
                    .thenByDescending { versionSequence(it.versionId) },
            )
            .toList()
    }

    @Synchronized
    fun readVersion(relativePath: String, versionId: String): String {
        val file = resolveVersion(relativePath, versionId)
        require(file.isFile) { "Note version does not exist" }
        return file.readText()
    }

    @Synchronized
    fun restoreVersion(relativePath: String, versionId: String): NoteFile {
        return save(relativePath, readVersion(relativePath, versionId))
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
        require(isWithinRoot(file, root)) { "Path escapes notes root" }
        return file
    }

    fun resolveNote(relativePath: String): File {
        val normalized = normalizePath(relativePath)
        val file = File(notesRoot, normalized).canonicalFile
        val root = notesRoot.canonicalFile
        require(isWithinRoot(file, root)) { "Note path escapes notes root" }
        return file
    }

    private fun createBackup(relativePath: String, contents: ByteArray) {
        val directory = resolveBackupDirectory(relativePath)
        check(directory.mkdirs() || directory.isDirectory) { "Failed to create note backup directory" }
        val timestamp = clock().coerceAtLeast(0L)
        var sequence = 0
        var target: File
        do {
            target = File(directory, "$timestamp-$sequence$VERSION_FILE_SUFFIX")
            sequence++
        } while (target.exists())
        writeDurably(target, contents)
        pruneBackups(directory)
    }

    private fun pruneBackups(noteDirectory: File) {
        versionFiles(noteDirectory)
            .sortedWith(versionFileComparator)
            .drop(maxVersionsPerNote)
            .forEach { check(it.delete()) { "Failed to prune note backup ${it.name}" } }

        versionFiles(backupsRoot)
            .sortedWith(versionFileComparator)
            .drop(maxVersionsTotal)
            .forEach { check(it.delete()) { "Failed to prune note backup ${it.name}" } }
        removeEmptyBackupDirectories()
    }

    private fun versionFiles(root: File): List<File> {
        if (!root.isDirectory) return emptyList()
        return root.walkTopDown()
            .filter { it.isFile && VERSION_FILE_PATTERN.matches(it.name) }
            .toList()
    }

    private fun removeEmptyBackupDirectories() {
        if (!backupsRoot.isDirectory) return
        backupsRoot.walkBottomUp()
            .filter { it.isDirectory && it != backupsRoot && it.list().orEmpty().isEmpty() }
            .forEach { it.delete() }
    }

    private fun resolveBackupDirectory(relativePath: String): File {
        val normalized = normalizePath(relativePath)
        val directory = File(backupsRoot, normalized).canonicalFile
        val root = backupsRoot.canonicalFile
        require(isWithinRoot(directory, root) && directory != root) { "Backup path escapes backup root" }
        return directory
    }

    private fun resolveVersion(relativePath: String, versionId: String): File {
        require(VERSION_FILE_PATTERN.matches(versionId)) { "Invalid note version ID" }
        val directory = resolveBackupDirectory(relativePath)
        val file = File(directory, versionId).canonicalFile
        require(isWithinRoot(file, directory) && file != directory) { "Version path escapes note backup directory" }
        return file
    }

    private fun writeDurably(target: File, contents: ByteArray) {
        val parent = requireNotNull(target.parentFile) { "File has no parent directory" }
        check(parent.mkdirs() || parent.isDirectory) { "Failed to create ${parent.path}" }
        val temporary = File(parent, ".${target.name}.${System.nanoTime()}.tmp")
        try {
            FileOutputStream(temporary).use { output ->
                output.write(contents)
                output.flush()
                output.fd.sync()
            }
            try {
                Files.move(
                    temporary.toPath(),
                    target.toPath(),
                    StandardCopyOption.ATOMIC_MOVE,
                    StandardCopyOption.REPLACE_EXISTING,
                )
            } catch (_: AtomicMoveNotSupportedException) {
                Files.move(temporary.toPath(), target.toPath(), StandardCopyOption.REPLACE_EXISTING)
            }
        } finally {
            temporary.delete()
        }
    }

    companion object {
        const val NOTES_DIR = "notes"
        const val NOTE_BACKUPS_DIR = "note_backups"
        const val DEFAULT_MAX_VERSIONS_PER_NOTE = 25
        const val DEFAULT_MAX_VERSIONS_TOTAL = 500
        private const val VERSION_FILE_SUFFIX = ".bak"
        private val VERSION_FILE_PATTERN = Regex("^[0-9]+-[0-9]+\\.bak$")
        private val versionFileComparator = compareByDescending<File> { versionTimestamp(it.name) }
            .thenByDescending { versionSequence(it.name) }
            .thenByDescending { it.path }

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

        fun buildNewNotePath(folder: String, input: String): String {
            val normalizedFolder = folder
                .trim()
                .replace('\\', '/')
                .split('/')
                .filter { it.isNotBlank() && it != "." && it != ".." }
                .joinToString("/")
            val cleanInput = input.trim().replace('\\', '/')
            val combined = listOf(normalizedFolder, cleanInput)
                .filter { it.isNotBlank() }
                .joinToString("/")
            return normalizePath(combined)
        }

        fun collectFolders(notePaths: List<String>, directoryPaths: List<String> = emptyList()): List<String> {
            val folders = linkedSetOf<String>()
            notePaths.forEach { path ->
                val parts = normalizedPathParts(path)
                parts.dropLast(1).indices.forEach { index ->
                    folders += parts.take(index + 1).joinToString("/")
                }
            }
            directoryPaths.forEach { path ->
                val parts = normalizedPathParts(path)
                parts.indices.forEach { index ->
                    folders += parts.take(index + 1).joinToString("/")
                }
            }
            return folders.sortedWith(String.CASE_INSENSITIVE_ORDER)
        }

        private fun normalizedPathParts(input: String): List<String> {
            return input
                .trim()
                .replace('\\', '/')
                .split('/')
                .filter { it.isNotBlank() && it != "." && it != ".." }
        }

        private fun versionTimestamp(versionId: String): Long {
            return versionId.substringBefore('-').toLongOrNull() ?: 0L
        }

        private fun versionSequence(versionId: String): Int {
            return versionId.substringAfter('-').substringBefore('.').toIntOrNull() ?: 0
        }

        private fun isWithinRoot(file: File, root: File): Boolean {
            return file.path == root.path || file.path.startsWith(root.path + File.separator)
        }

        fun isManagedAssetDirName(name: String): Boolean = name == "assets" || name.endsWith(".assets")
    }
}

internal fun completeLocalNoteDeletion(
    deleteLocal: () -> Boolean,
    afterDelete: () -> Unit,
): Boolean {
    if (!deleteLocal()) return false
    afterDelete()
    return true
}

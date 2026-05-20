package com.kloneets.kokotools

data class NotePickerRow(
    val label: String,
    val relativePath: String,
    val depth: Int,
    val folder: Boolean,
)

object NotePickerRows {
    fun flat(notes: List<NoteFile>): List<NotePickerRow> {
        return notes.map {
            NotePickerRow(
                label = it.relativePath,
                relativePath = it.relativePath,
                depth = 0,
                folder = false,
            )
        }
    }

    fun tree(notes: List<NoteFile>, expandedFolders: Set<String>): List<NotePickerRow> {
        val root = FolderNode("")
        notes.forEach { note ->
            val parts = note.relativePath.split('/').filter { it.isNotBlank() }
            if (parts.isEmpty()) return@forEach
            var folder = root
            parts.dropLast(1).forEach { part ->
                val childPath = if (folder.path.isBlank()) part else "${folder.path}/$part"
                folder = folder.folders.getOrPut(part) { FolderNode(childPath) }
            }
            folder.notes.add(note.relativePath)
        }

        return buildRows(root, expandedFolders, depth = 0)
    }

    private fun buildRows(
        folder: FolderNode,
        expandedFolders: Set<String>,
        depth: Int,
    ): List<NotePickerRow> {
        val rows = mutableListOf<NotePickerRow>()
        folder.folders.toSortedMap(String.CASE_INSENSITIVE_ORDER).forEach { (name, child) ->
            val expanded = expandedFolders.contains(child.path)
            rows.add(
                NotePickerRow(
                    label = "${if (expanded) "[-]" else "[+]"} $name",
                    relativePath = child.path,
                    depth = depth,
                    folder = true,
                ),
            )
            if (expanded) {
                rows.addAll(buildRows(child, expandedFolders, depth + 1))
            }
        }
        folder.notes
            .sortedWith(String.CASE_INSENSITIVE_ORDER)
            .forEach { notePath ->
                rows.add(
                    NotePickerRow(
                        label = notePath.substringAfterLast('/'),
                        relativePath = notePath,
                        depth = depth,
                        folder = false,
                    ),
                )
            }
        return rows
    }

    private data class FolderNode(
        val path: String,
        val folders: MutableMap<String, FolderNode> = linkedMapOf(),
        val notes: MutableList<String> = mutableListOf(),
    )
}

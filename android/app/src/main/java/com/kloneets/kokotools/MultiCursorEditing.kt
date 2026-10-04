package com.kloneets.kokotools

/** UTF-16 ranges, matching Android EditText selection offsets. */
data class MultiCursorRange(val start: Int, val end: Int) {
    val isEmpty: Boolean get() = start == end
}

/** Small editor model kept separate from views so range behavior can be tested. */
class MultiCursorEditing {
    var ranges: List<MultiCursorRange> = emptyList()
        private set
    var primaryIndex: Int = -1
        private set
    val active: Boolean get() = ranges.isNotEmpty()

    fun begin(start: Int, end: Int) {
        val range = MultiCursorRange(minOf(start, end), maxOf(start, end))
        ranges = listOf(range)
        primaryIndex = 0
    }

    fun beginAtText(text: String, start: Int, end: Int) {
        var rangeStart = minOf(start, end).coerceIn(0, text.length)
        var rangeEnd = maxOf(start, end).coerceIn(0, text.length)
        if (rangeStart == rangeEnd) {
            while (rangeStart > 0) {
                val codePoint = Character.codePointBefore(text, rangeStart)
                if (!codePoint.isWordCodePoint()) break
                rangeStart -= Character.charCount(codePoint)
            }
            while (rangeEnd < text.length) {
                val codePoint = Character.codePointAt(text, rangeEnd)
                if (!codePoint.isWordCodePoint()) break
                rangeEnd += Character.charCount(codePoint)
            }
        }
        begin(rangeStart, rangeEnd)
    }

    fun clear() {
        ranges = emptyList()
        primaryIndex = -1
    }

    fun setCollapsedRanges(positions: List<Int>, primaryPosition: Int) {
        val unique = positions.distinct().sorted()
        ranges = unique.map { MultiCursorRange(it, it) }
        primaryIndex = ranges.indexOfFirst { it.start == primaryPosition }.coerceAtLeast(0)
    }

    fun setSelections(selections: List<MultiCursorRange>, selectedPrimaryIndex: Int) {
        if (selections.isEmpty()) {
            clear()
            return
        }
        val primary = selections[selectedPrimaryIndex.coerceIn(0, selections.lastIndex)]
        val normalized = selections.map { MultiCursorRange(minOf(it.start, it.end), maxOf(it.start, it.end)) }
            .sortedWith(compareBy<MultiCursorRange> { it.start }.thenBy { it.end })
        val merged = mutableListOf<MultiCursorRange>()
        normalized.forEach { range ->
            val last = merged.lastOrNull()
            if (last != null && range.start < last.end) {
                merged[merged.lastIndex] = MultiCursorRange(last.start, maxOf(last.end, range.end))
            } else if (last != range) {
                merged += range
            }
        }
        ranges = merged
        primaryIndex = ranges.indexOfFirst { primary.start >= it.start && primary.end <= it.end }.coerceAtLeast(0)
    }

    fun promoteSelection(index: Int, replacement: MultiCursorRange? = null) {
        if (ranges.isEmpty()) return
        val safeIndex = index.coerceIn(0, ranges.lastIndex)
        val next = ranges.toMutableList()
        if (replacement != null) next[safeIndex] = replacement
        setSelections(next, safeIndex)
    }

    /** Selects the next exact occurrence, using UTF-16 offsets and no wrapping. */
    fun addNext(text: String, selectionStart: Int, selectionEnd: Int): MultiCursorRange? {
        if (!active) begin(selectionStart, selectionEnd)
        val selectedNow = MultiCursorRange(
            minOf(selectionStart, selectionEnd).coerceIn(0, text.length),
            maxOf(selectionStart, selectionEnd).coerceIn(0, text.length),
        )
        val retained = ranges.filterIndexed { index, range ->
            if (index == primaryIndex) return@filterIndexed false
            val overlaps = range == selectedNow ||
                (range.start < selectedNow.end && selectedNow.start < range.end) ||
                (range.isEmpty && range.start in selectedNow.start..selectedNow.end) ||
                (selectedNow.isEmpty && selectedNow.start in range.start..range.end)
            !overlaps
        }
        ranges = (retained + selectedNow).sortedBy { it.start }
        primaryIndex = ranges.indexOf(selectedNow)
        val primary = ranges[primaryIndex]
        var queryStart = primary.start
        var queryEnd = primary.end
        if (queryStart == queryEnd) {
            while (queryStart > 0) {
                val codePoint = Character.codePointBefore(text, queryStart)
                if (!codePoint.isWordCodePoint()) break
                queryStart -= Character.charCount(codePoint)
            }
            while (queryEnd < text.length) {
                val codePoint = Character.codePointAt(text, queryEnd)
                if (!codePoint.isWordCodePoint()) break
                queryEnd += Character.charCount(codePoint)
            }
            if (queryStart < queryEnd) {
                val wordRange = MultiCursorRange(queryStart, queryEnd)
                val withoutPrimary = ranges.filterIndexed { index, _ -> index != primaryIndex }
                ranges = (withoutPrimary + wordRange).sortedBy { it.start }
                primaryIndex = ranges.indexOf(wordRange)
            }
        }
        if (queryStart == queryEnd) return null
        val query = text.substring(queryStart, queryEnd)
        var offset = maxOf(queryEnd, selectionEnd.coerceAtLeast(0))
        while (offset <= text.length - query.length) {
            val found = text.indexOf(query, offset)
            if (found < 0) return null
            val candidate = MultiCursorRange(found, found + query.length)
            if (ranges.none { it == candidate || it.start < candidate.end && candidate.start < it.end }) {
                ranges = (ranges + candidate).sortedBy { it.start }
                primaryIndex = ranges.indexOf(candidate)
                return candidate
            }
            offset = found + query.length.coerceAtLeast(1)
        }
        return null
    }

    fun replaceAll(text: String, replacement: String): String {
        if (!active) return text
        val builder = StringBuilder(text)
        ranges.sortedByDescending { it.start }.forEach { range ->
            builder.replace(range.start, range.end, replacement)
        }
        val updated = ranges.map { range ->
            val priorDelta = ranges.filter { it.start < range.start }.sumOf { replacement.length - (it.end - it.start) }
            val start = range.start + priorDelta
            MultiCursorRange(start, start + replacement.length)
        }
        ranges = updated.sortedBy { it.start }
        primaryIndex = ranges.indexOfFirst { it.start == updated.getOrNull(primaryIndex)?.start }
        return builder.toString()
    }
}

private fun Int.isWordCodePoint(): Boolean = Character.isLetterOrDigit(this) || this == '_'.code

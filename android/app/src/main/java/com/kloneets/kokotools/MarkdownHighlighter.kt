package com.kloneets.kokotools

import android.graphics.Color
import android.graphics.Typeface
import android.text.Editable
import android.text.Spannable
import android.text.TextWatcher
import android.text.style.BackgroundColorSpan
import android.text.style.ForegroundColorSpan
import android.text.style.StyleSpan
import android.text.style.TypefaceSpan

data class MarkdownHighlight(
    val start: Int,
    val end: Int,
    val type: MarkdownHighlightType,
)

data class FencedCodeBlock(
    val start: Int,
    val end: Int,
    val contentStart: Int,
    val contentEnd: Int,
    val language: String,
)

enum class MarkdownHighlightType {
    Heading,
    Bold,
    Italic,
    InlineCode,
    CodeBlock,
    Link,
    Quote,
    List,
    Checkbox,
    HorizontalRule,
    Table,
}

class MarkdownHighlighter(
    private val theme: Theme,
) {
    data class Theme(
        val heading: Int,
        val bold: Int,
        val italic: Int,
        val link: Int,
        val quote: Int,
        val list: Int,
        val checkbox: Int,
        val horizontalRule: Int,
        val table: Int,
        val codeText: Int,
        val codeBackground: Int,
        val codeKeyword: Int,
        val codeString: Int,
        val codeNumber: Int,
        val codeComment: Int,
        val codeOperator: Int,
        val codePunctuation: Int,
        val codeType: Int,
        val codeProperty: Int,
        val codeFunction: Int,
        val codeLiteral: Int,
        val codeTag: Int,
        val codeAttribute: Int,
    )

    fun apply(editable: Editable) {
        clearExistingSpans(editable)
        val markdown = editable.toString()
        for (highlight in findHighlights(markdown)) {
            applyHighlight(editable, highlight)
        }
        applyCodeSyntax(editable, markdown)
    }

    private fun clearExistingSpans(editable: Editable) {
        editable.getSpans(0, editable.length, MarkdownSpan::class.java).forEach { span ->
            editable.removeSpan(span)
        }
    }

    private fun applyHighlight(editable: Editable, highlight: MarkdownHighlight) {
        if (highlight.start >= highlight.end) return
        val spans = when (highlight.type) {
            MarkdownHighlightType.Heading -> listOf(
                MarkdownForegroundSpan(theme.heading),
                MarkdownStyleSpan(Typeface.BOLD),
            )
            MarkdownHighlightType.Bold -> listOf(
                MarkdownForegroundSpan(theme.bold),
                MarkdownStyleSpan(Typeface.BOLD),
            )
            MarkdownHighlightType.Italic -> listOf(
                MarkdownForegroundSpan(theme.italic),
                MarkdownStyleSpan(Typeface.ITALIC),
            )
            MarkdownHighlightType.InlineCode,
            MarkdownHighlightType.CodeBlock -> listOf(
                MarkdownForegroundSpan(theme.codeText),
                MarkdownBackgroundSpan(theme.codeBackground),
                MarkdownTypefaceSpan("monospace"),
            )
            MarkdownHighlightType.Link -> listOf(MarkdownForegroundSpan(theme.link))
            MarkdownHighlightType.Quote -> listOf(MarkdownForegroundSpan(theme.quote))
            MarkdownHighlightType.List -> listOf(MarkdownForegroundSpan(theme.list))
            MarkdownHighlightType.Checkbox -> listOf(MarkdownForegroundSpan(theme.checkbox))
            MarkdownHighlightType.HorizontalRule -> listOf(MarkdownForegroundSpan(theme.horizontalRule))
            MarkdownHighlightType.Table -> listOf(MarkdownForegroundSpan(theme.table))
        }
        spans.forEach { span ->
            editable.setSpan(span, highlight.start, highlight.end, Spannable.SPAN_EXCLUSIVE_EXCLUSIVE)
        }
    }

    private fun applyCodeSyntax(editable: Editable, markdown: String) {
        for (block in findCodeBlocks(markdown)) {
            if (block.language.isBlank() || block.contentStart >= block.contentEnd) continue
            val code = markdown.substring(block.contentStart, block.contentEnd)
            for (token in CodeSyntaxHighlighter.tokenize(block.language, code)) {
                val start = block.contentStart + token.start
                val end = block.contentStart + token.end
                if (start < end && start >= block.contentStart && end <= block.contentEnd) {
                    applyCodeToken(editable, start, end, token.type)
                }
            }
        }
    }

    private fun applyCodeToken(editable: Editable, start: Int, end: Int, type: String?) {
        val color = codeTokenColor(type) ?: return
        editable.setSpan(MarkdownForegroundSpan(color), start, end, Spannable.SPAN_EXCLUSIVE_EXCLUSIVE)
        if (type == "comment") {
            editable.setSpan(MarkdownStyleSpan(Typeface.ITALIC), start, end, Spannable.SPAN_EXCLUSIVE_EXCLUSIVE)
        }
    }

    private fun codeTokenColor(type: String?): Int? {
        return when (type) {
            "keyword", "important", "atrule", "selector" -> theme.codeKeyword
            "string", "char", "regex", "inserted" -> theme.codeString
            "number" -> theme.codeNumber
            "comment", "prolog", "doctype", "cdata" -> theme.codeComment
            "operator", "entity", "url" -> theme.codeOperator
            "punctuation" -> theme.codePunctuation
            "class-name", "builtin", "namespace" -> theme.codeType
            "property", "constant", "symbol" -> theme.codeProperty
            "function", "method" -> theme.codeFunction
            "boolean", "null", "nil", "deleted" -> theme.codeLiteral
            "tag" -> theme.codeTag
            "attr-name", "attr-value" -> theme.codeAttribute
            else -> null
        }
    }

    interface MarkdownSpan

    private class MarkdownForegroundSpan(color: Int) : ForegroundColorSpan(color), MarkdownSpan
    private class MarkdownBackgroundSpan(color: Int) : BackgroundColorSpan(color), MarkdownSpan
    private class MarkdownStyleSpan(style: Int) : StyleSpan(style), MarkdownSpan
    private class MarkdownTypefaceSpan(family: String) : TypefaceSpan(family), MarkdownSpan

    companion object {
        fun default(): MarkdownHighlighter {
            return MarkdownHighlighter(
                Theme(
                    heading = Color.rgb(168, 78, 0),
                    bold = Color.rgb(176, 112, 0),
                    italic = Color.rgb(170, 56, 124),
                    link = Color.rgb(0, 106, 150),
                    quote = Color.rgb(118, 74, 166),
                    list = Color.rgb(18, 132, 88),
                    checkbox = Color.rgb(0, 128, 148),
                    horizontalRule = Color.rgb(151, 79, 62),
                    table = Color.rgb(157, 103, 12),
                    codeText = Color.rgb(235, 240, 218),
                    codeBackground = Color.rgb(54, 62, 43),
                    codeKeyword = Color.rgb(255, 133, 161),
                    codeString = Color.rgb(166, 226, 104),
                    codeNumber = Color.rgb(174, 150, 255),
                    codeComment = Color.rgb(148, 160, 124),
                    codeOperator = Color.rgb(255, 197, 92),
                    codePunctuation = Color.rgb(213, 220, 190),
                    codeType = Color.rgb(116, 214, 255),
                    codeProperty = Color.rgb(255, 171, 92),
                    codeFunction = Color.rgb(96, 222, 205),
                    codeLiteral = Color.rgb(213, 137, 255),
                    codeTag = Color.rgb(255, 121, 121),
                    codeAttribute = Color.rgb(133, 199, 255),
                ),
            )
        }

        fun findHighlights(markdown: String): List<MarkdownHighlight> {
            if (markdown.isEmpty()) return emptyList()

            val highlights = mutableListOf<MarkdownHighlight>()
            val codeBlocks = findCodeBlocks(markdown)
            val codeRanges = codeBlocks.map { it.start until it.end }
            highlights.addAll(codeBlocks.map { MarkdownHighlight(it.start, it.end, MarkdownHighlightType.CodeBlock) })

            val lineRanges = markdown.lineRanges()
            val tableLines = tableLineIndexes(markdown, lineRanges)
            lineRanges.forEachIndexed { index, range ->
                val line = markdown.substring(range)
                val lineContent = line.trimEnd('\n', '\r')
                val contentEnd = lineContent.length
                val end = range.first + contentEnd
                if (range.first >= end || codeRanges.any { it.overlaps(range.first, end) }) return@forEachIndexed

                when {
                    HEADING_REGEX.matches(lineContent) -> highlights.add(MarkdownHighlight(range.first, end, MarkdownHighlightType.Heading))
                    CHECKBOX_REGEX.matches(lineContent) -> highlights.add(MarkdownHighlight(range.first, end, MarkdownHighlightType.Checkbox))
                    LIST_REGEX.matches(lineContent) -> highlights.add(MarkdownHighlight(range.first, end, MarkdownHighlightType.List))
                    QUOTE_REGEX.matches(lineContent) -> highlights.add(MarkdownHighlight(range.first, end, MarkdownHighlightType.Quote))
                    HORIZONTAL_RULE_REGEX.matches(lineContent) -> highlights.add(MarkdownHighlight(range.first, end, MarkdownHighlightType.HorizontalRule))
                    index in tableLines -> highlights.add(MarkdownHighlight(range.first, end, MarkdownHighlightType.Table))
                }
            }

            val protectedRanges = codeRanges.toMutableList()
            for (match in INLINE_CODE_REGEX.findAll(markdown)) {
                if (protectedRanges.any { it.overlaps(match.range.first, match.range.last + 1) }) continue
                protectedRanges.add(match.range)
                highlights.add(MarkdownHighlight(match.range.first, match.range.last + 1, MarkdownHighlightType.InlineCode))
            }

            addInlineHighlights(markdown, LINK_REGEX, MarkdownHighlightType.Link, protectedRanges, highlights)
            addInlineHighlights(markdown, BOLD_REGEX, MarkdownHighlightType.Bold, protectedRanges, highlights)
            addInlineHighlights(markdown, ITALIC_REGEX, MarkdownHighlightType.Italic, protectedRanges, highlights)

            return highlights.sortedWith(compareBy<MarkdownHighlight> { it.start }.thenBy { it.end }.thenBy { it.type.name })
        }

        fun normalizeCodeLanguage(language: String): String {
            val normalized = language.trim()
                .substringBefore(' ')
                .substringBefore('\t')
                .removePrefix("language-")
                .removePrefix("lang-")
                .lowercase()
            return when (normalized) {
                "kt", "kts" -> "kotlin"
                "js", "jsx", "mjs", "cjs" -> "javascript"
                "html", "xml", "svg" -> "markup"
                "yml" -> "yaml"
                "md" -> "markdown"
                "golang" -> "go"
                "py" -> "python"
                else -> normalized
            }
        }

        fun findCodeBlocks(markdown: String): List<FencedCodeBlock> {
            val blocks = mutableListOf<FencedCodeBlock>()
            var blockStart = -1
            var contentStart = -1
            var fenceMarker = ""
            var language = ""

            for (range in markdown.lineRanges()) {
                val line = markdown.substring(range)
                val match = FENCE_REGEX.find(line)
                if (match == null) continue

                val marker = match.groupValues[1].first().toString()
                if (blockStart < 0) {
                    blockStart = range.first
                    contentStart = range.last + 1
                    fenceMarker = marker
                    language = normalizeCodeLanguage(match.groupValues.getOrElse(2) { "" })
                } else if (marker == fenceMarker) {
                    val blockEnd = range.first + line.trimEnd('\n', '\r').length
                    blocks.add(
                        FencedCodeBlock(
                            start = blockStart,
                            end = blockEnd,
                            contentStart = contentStart.coerceIn(blockStart, blockEnd),
                            contentEnd = range.first,
                            language = language,
                        ),
                    )
                    blockStart = -1
                    contentStart = -1
                    fenceMarker = ""
                    language = ""
                }
            }

            if (blockStart >= 0) {
                blocks.add(
                    FencedCodeBlock(
                        start = blockStart,
                        end = markdown.length,
                        contentStart = contentStart.coerceIn(blockStart, markdown.length),
                        contentEnd = markdown.length,
                        language = language,
                    ),
                )
            }
            return blocks
        }

        private fun tableLineIndexes(markdown: String, lineRanges: List<IntRange>): Set<Int> {
            val tableLines = mutableSetOf<Int>()
            lineRanges.forEachIndexed { index, range ->
                val line = markdown.substring(range).trim()
                if (!TABLE_SEPARATOR_REGEX.matches(line)) return@forEachIndexed

                val previous = index - 1
                if (previous >= 0 && markdown.substring(lineRanges[previous]).contains('|')) {
                    tableLines.add(previous)
                    tableLines.add(index)
                    var next = index + 1
                    while (next < lineRanges.size && markdown.substring(lineRanges[next]).contains('|')) {
                        tableLines.add(next)
                        next += 1
                    }
                }
            }
            return tableLines
        }

        private fun addInlineHighlights(
            markdown: String,
            regex: Regex,
            type: MarkdownHighlightType,
            protectedRanges: List<IntRange>,
            highlights: MutableList<MarkdownHighlight>,
        ) {
            for (match in regex.findAll(markdown)) {
                val end = match.range.last + 1
                if (protectedRanges.any { it.overlaps(match.range.first, end) }) continue
                highlights.add(MarkdownHighlight(match.range.first, end, type))
            }
        }

        private fun String.lineRanges(): List<IntRange> {
            val ranges = mutableListOf<IntRange>()
            var start = 0
            while (start < length) {
                val nextBreak = indexOf('\n', start)
                val endExclusive = if (nextBreak < 0) length else nextBreak + 1
                ranges.add(start until endExclusive)
                start = endExclusive
            }
            return ranges
        }

        private fun IntRange.overlaps(start: Int, end: Int): Boolean {
            return first < end && start < last + 1
        }

        private val HEADING_REGEX = Regex("""^\s{0,3}#{1,6}\s+.+$""")
        private val CHECKBOX_REGEX = Regex("""^\s{0,3}(?:[-+*]|\d+[.)])\s+\[[ xX]\]\s+.+$""")
        private val LIST_REGEX = Regex("""^\s{0,3}(?:[-+*]|\d+[.)])\s+.+$""")
        private val QUOTE_REGEX = Regex("""^\s{0,3}>\s?.+$""")
        private val HORIZONTAL_RULE_REGEX = Regex("""^\s{0,3}(?:[-*_]\s*){3,}$""")
        private val FENCE_REGEX = Regex("""^\s{0,3}(`{3,}|~{3,})\s*([^\s`]*)?.*$""")
        private val INLINE_CODE_REGEX = Regex("""(?<!`)`[^`\n]+`(?!`)""")
        private val LINK_REGEX = Regex("""\[[^\]\n]+]\([^) \n]+(?:\s+"[^"\n]+")?\)""")
        private val BOLD_REGEX = Regex("""(?<!\*)\*\*[^*\n]+?\*\*(?!\*)|(?<!_)__[^_\n]+?__(?!_)""")
        private val ITALIC_REGEX = Regex("""(?<![*\w])\*[^*\n]+?\*(?!\*)|(?<![_\w])_[^_\n]+?_(?!_)""")
        private val TABLE_SEPARATOR_REGEX = Regex("""^\|?\s*:?-{3,}:?\s*(?:\|\s*:?-{3,}:?\s*)+\|?\s*$""")
    }
}

class MarkdownHighlightingTextWatcher(
    private val highlighter: MarkdownHighlighter = MarkdownHighlighter.default(),
) : TextWatcher {
    private var highlighting = false

    override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
    override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) = Unit

    override fun afterTextChanged(s: Editable?) {
        if (s == null || highlighting) return
        highlighting = true
        try {
            highlighter.apply(s)
        } finally {
            highlighting = false
        }
    }
}

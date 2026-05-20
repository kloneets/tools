package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MarkdownHighlighterTest {
    @Test
    fun highlightsCommonInlineMarkdown() {
        val markdown = "# Title\nUse **bold**, *italic*, `code`, and [link](https://example.com)."
        val highlights = MarkdownHighlighter.findHighlights(markdown)

        assertHighlight(markdown, highlights, "# Title", MarkdownHighlightType.Heading)
        assertHighlight(markdown, highlights, "**bold**", MarkdownHighlightType.Bold)
        assertHighlight(markdown, highlights, "*italic*", MarkdownHighlightType.Italic)
        assertHighlight(markdown, highlights, "`code`", MarkdownHighlightType.InlineCode)
        assertHighlight(markdown, highlights, "[link](https://example.com)", MarkdownHighlightType.Link)
    }

    @Test
    fun highlightsFencedCodeBlocksAcrossLines() {
        val markdown = "before\n```kotlin\nval text = **not bold**\n```\nafter"
        val highlights = MarkdownHighlighter.findHighlights(markdown)
        val codeBlock = "```kotlin\nval text = **not bold**\n```"

        assertHighlight(markdown, highlights, codeBlock, MarkdownHighlightType.CodeBlock)
        assertFalse(highlights.any { it.type == MarkdownHighlightType.Bold })
    }

    @Test
    fun detectsFencedCodeBlockLanguageAndContentRange() {
        val markdown = "before\n```js\nconst count = 1\n```\nafter"
        val block = MarkdownHighlighter.findCodeBlocks(markdown).single()

        assertEquals("javascript", block.language)
        assertEquals(markdown.indexOf("const"), block.contentStart)
        assertEquals(markdown.indexOf("```", markdown.indexOf("const")), block.contentEnd)
    }

    @Test
    fun normalizesCommonCodeLanguageAliases() {
        assertEquals("kotlin", MarkdownHighlighter.normalizeCodeLanguage("kt"))
        assertEquals("javascript", MarkdownHighlighter.normalizeCodeLanguage("language-js"))
        assertEquals("markup", MarkdownHighlighter.normalizeCodeLanguage("html"))
        assertEquals("yaml", MarkdownHighlighter.normalizeCodeLanguage("yml"))
    }

    @Test
    fun tokenizesJsonCodeWithPrism() {
        val code = """{"enabled": true, "count": 3}"""
        val tokens = CodeSyntaxHighlighter.tokenize("json", code)

        assertTrue(tokens.any { it.type == "property" && code.substring(it.start, it.end) == """"enabled"""" })
        assertTrue(tokens.any { it.type == "boolean" && code.substring(it.start, it.end) == "true" })
        assertTrue(tokens.any { it.type == "number" && code.substring(it.start, it.end) == "3" })
    }

    @Test
    fun tokenizesKotlinCodeWithPrism() {
        val code = "val answer = 42 // comment"
        val tokens = CodeSyntaxHighlighter.tokenize("kotlin", code)

        assertTrue(tokens.any { it.type == "keyword" && code.substring(it.start, it.end) == "val" })
        assertTrue(tokens.any { it.type == "number" && code.substring(it.start, it.end) == "42" })
        assertTrue(tokens.any { it.type == "comment" && code.substring(it.start, it.end).contains("comment") })
    }

    @Test
    fun unknownCodeLanguageFallsBackWithoutTokens() {
        assertTrue(CodeSyntaxHighlighter.tokenize("made-up-language", "anything").isEmpty())
    }

    @Test
    fun highlightsBlockMarkdown() {
        val markdown = """
            > quote
            - bullet
            1. ordered
            - [x] checkbox
            ---
        """.trimIndent()
        val highlights = MarkdownHighlighter.findHighlights(markdown)

        assertHighlight(markdown, highlights, "> quote", MarkdownHighlightType.Quote)
        assertHighlight(markdown, highlights, "- bullet", MarkdownHighlightType.List)
        assertHighlight(markdown, highlights, "1. ordered", MarkdownHighlightType.List)
        assertHighlight(markdown, highlights, "- [x] checkbox", MarkdownHighlightType.Checkbox)
        assertHighlight(markdown, highlights, "---", MarkdownHighlightType.HorizontalRule)
    }

    @Test
    fun highlightsTables() {
        val markdown = """
            | Name | Value |
            | ---- | ----: |
            | One | 1 |
        """.trimIndent()
        val highlights = MarkdownHighlighter.findHighlights(markdown)

        assertHighlight(markdown, highlights, "| Name | Value |", MarkdownHighlightType.Table)
        assertHighlight(markdown, highlights, "| ---- | ----: |", MarkdownHighlightType.Table)
        assertHighlight(markdown, highlights, "| One | 1 |", MarkdownHighlightType.Table)
    }

    @Test
    fun skipsInlineHighlightsInsideInlineCode() {
        val markdown = "`**not bold** [not a link](https://example.com)` and **bold**"
        val highlights = MarkdownHighlighter.findHighlights(markdown)

        assertHighlight(markdown, highlights, "`**not bold** [not a link](https://example.com)`", MarkdownHighlightType.InlineCode)
        assertHighlight(markdown, highlights, "**bold**", MarkdownHighlightType.Bold)
        assertFalse(
            highlights.any {
                it.type == MarkdownHighlightType.Link ||
                    (it.type == MarkdownHighlightType.Bold && markdown.substring(it.start, it.end) == "**not bold**")
            },
        )
    }

    private fun assertHighlight(
        markdown: String,
        highlights: List<MarkdownHighlight>,
        token: String,
        type: MarkdownHighlightType,
    ) {
        val start = markdown.indexOf(token)
        assertTrue("Token not found: $token", start >= 0)
        val end = start + token.length
        assertEquals(
            "$type range for $token",
            MarkdownHighlight(start, end, type),
            highlights.firstOrNull { it.start == start && it.end == end && it.type == type },
        )
    }
}

package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MarkdownDocumentModelTest {
    @Test
    fun marksOnlyCursorLineActive() {
        val markdown = "# Title\nsecond **line**\nthird"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second"))

        assertEquals(3, segments.size)
        assertFalse(segments[0].active)
        assertTrue(segments[1].active)
        assertFalse(segments[2].active)
        assertEquals("# Title", segments[0].markdownText)
        assertEquals("second **line**\n", segments[1].markdownText)
        assertEquals("third", segments[2].markdownText)
    }

    @Test
    fun inactiveRenderedSegmentsDoNotKeepSourceLineBreaks() {
        val markdown = "first\nsecond\nthird"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second"))

        assertEquals("first", segments[0].markdownText)
        assertEquals("second\n", segments[1].markdownText)
        assertEquals("third", segments[2].markdownText)
    }

    @Test
    fun keepsInactiveMarkdownLinesSeparatelyTargetable() {
        val markdown = "first\nsecond\nthird"
        val segments = MarkdownDocumentModel.segments(markdown, 0)

        assertEquals(3, segments.size)
        assertTrue(segments[0].active)
        assertEquals("second", segments[1].markdownText)
        assertEquals("third", segments[2].markdownText)
    }

    @Test
    fun listNeighborsAroundActiveListLineRenderAsSource() {
        val markdown = "- first\n- second\n- third"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second"))

        assertEquals(3, segments.size)
        assertEquals("- first", segments[0].markdownText)
        assertTrue(segments[0].renderAsSource)
        assertTrue(segments[1].active)
        assertEquals("- third", segments[2].markdownText)
        assertTrue(segments[2].renderAsSource)
    }

    @Test
    fun nestedListNeighborsAroundActiveNestedListLineRenderAsSource() {
        val markdown = "- first\n    - second\n    - third\n- fourth"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second"))

        assertEquals(4, segments.size)
        assertEquals("- first", segments[0].markdownText)
        assertTrue(segments[0].renderAsSource)
        assertTrue(segments[1].active)
        assertEquals("    - third", segments[2].markdownText)
        assertTrue(segments[2].renderAsSource)
        assertEquals("- fourth", segments[3].markdownText)
    }

    @Test
    fun tabIndentedListNeighborsAroundActiveNestedListLineRenderAsSource() {
        val markdown = "- first\n\t- second\n\t- third"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second"))

        assertEquals(3, segments.size)
        assertTrue(segments[0].renderAsSource)
        assertTrue(segments[1].active)
        assertTrue(segments[2].renderAsSource)
    }

    @Test
    fun nonListNeighborsAroundActiveListLineKeepRenderedMarkdown() {
        val markdown = "intro\n- second\noutro"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second"))

        assertFalse(segments[0].renderAsSource)
        assertTrue(segments[1].active)
        assertFalse(segments[2].renderAsSource)
    }

    @Test
    fun marksWholeFencedCodeBlockActiveWhenCursorIsInsideIt() {
        val markdown = "before\n```kotlin\nval answer = 42\n```\nafter"
        val segments = MarkdownDocumentModel.segments(markdown, markdown.indexOf("answer"))
        val active = segments.single { it.active }

        assertTrue(active.codeBlock)
        assertEquals("```kotlin\nval answer = 42\n```", active.markdownText)
    }

    @Test
    fun inactiveCodeBlockSegmentExcludesFenceMarkers() {
        val markdown = "before\n```json\n{\"count\": 1}\n```\nafter"
        val segments = MarkdownDocumentModel.segments(markdown, 0)
        val code = segments.single { it.codeBlock }

        assertFalse(code.active)
        assertEquals("json", code.language)
        assertEquals("{\"count\": 1}", code.markdownText)
    }

    @Test
    fun renderedOffsetMapsToRawOffsetForInactiveMarkdown() {
        val markdown = "first\nsecond\nthird"
        val segment = MarkdownDocumentModel.segments(markdown, markdown.indexOf("third"))[1]

        assertEquals(markdown.indexOf("second"), MarkdownDocumentModel.rawOffsetForRenderedOffset(segment, 0))
    }

    @Test
    fun renderedOffsetMapsToCodeContentInsteadOfFence() {
        val markdown = "before\n```json\n{\"count\": 1}\n```\nafter"
        val code = MarkdownDocumentModel.segments(markdown, 0).single { it.codeBlock }

        assertEquals(markdown.indexOf("{\"count\""), MarkdownDocumentModel.rawOffsetForRenderedOffset(code, 0))
        assertEquals(markdown.indexOf("1}"), MarkdownDocumentModel.rawOffsetForRenderedOffset(code, code.markdownText.length - 2))
    }

    @Test
    fun replacesActiveRangeWithoutChangingOtherMarkdown() {
        val markdown = "# Title\nold line\nthird"
        val range = MarkdownDocumentModel.activeRange(markdown, markdown.indexOf("old"))
        val updated = MarkdownDocumentModel.replaceRange(markdown, range, "new **line**\n")

        assertEquals("# Title\nnew **line**\nthird", updated)
    }

    @Test
    fun activeLineEditorHidesSourceLineBreakButKeepsItWhenSaving() {
        val markdown = "first\nsecond\nthird"
        val active = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second")).single { it.active }

        assertEquals("second", MarkdownDocumentModel.editableText(active))
        assertEquals("changed\n", MarkdownDocumentModel.replacementText(active, "changed"))
    }

    @Test
    fun activeLastLineEditorDoesNotAddLineBreakWhenSaving() {
        val markdown = "first\nsecond"
        val active = MarkdownDocumentModel.segments(markdown, markdown.indexOf("second")).single { it.active }

        assertEquals("second", MarkdownDocumentModel.editableText(active))
        assertEquals("changed", MarkdownDocumentModel.replacementText(active, "changed"))
    }

    @Test
    fun trailingEmptyLineCanBecomeActive() {
        val markdown = "first\n"
        val active = MarkdownDocumentModel.segments(markdown, markdown.length).single { it.active }

        assertEquals(markdown.length, active.rawStart)
        assertEquals(markdown.length, active.rawEnd)
        assertEquals("", MarkdownDocumentModel.editableText(active))
        assertEquals("second", MarkdownDocumentModel.replacementText(active, "second"))
        assertEquals("first\nsecond", MarkdownDocumentModel.replaceRange(markdown, active.rawStart until active.rawEnd, "second"))
    }

    @Test
    fun adjacentActiveOffsetMovesBetweenLines() {
        val markdown = "first\nsecond\nthird"
        val second = MarkdownDocumentModel.activeRange(markdown, markdown.indexOf("second"))

        assertEquals(markdown.indexOf("third"), MarkdownDocumentModel.adjacentActiveOffset(markdown, second, 1))
        assertEquals(markdown.indexOf("first"), MarkdownDocumentModel.adjacentActiveOffset(markdown, second, -1))
    }

    @Test
    fun adjacentActiveOffsetMovesToTrailingEmptyLine() {
        val markdown = "first\nsecond\n"
        val second = MarkdownDocumentModel.activeRange(markdown, markdown.indexOf("second"))

        assertEquals(markdown.length, MarkdownDocumentModel.adjacentActiveOffset(markdown, second, 1))
    }

    @Test
    fun adjacentActiveOffsetMovesAcrossCodeBlocks() {
        val markdown = "before\n```kotlin\nval answer = 42\n```\nafter"
        val code = MarkdownDocumentModel.activeRange(markdown, markdown.indexOf("answer"))

        assertEquals(markdown.indexOf("after"), MarkdownDocumentModel.adjacentActiveOffset(markdown, code, 1))
        assertEquals(markdown.indexOf("before"), MarkdownDocumentModel.adjacentActiveOffset(markdown, code, -1))
    }

    @Test
    fun joinWithPreviousLineRemovesLineBreakBeforeActiveLine() {
        val markdown = "first\nsecond\nthird"
        val second = MarkdownDocumentModel.activeRange(markdown, markdown.indexOf("second"))
        val joined = MarkdownDocumentModel.joinWithPreviousLine(markdown, second)

        assertEquals("firstsecond\nthird", joined?.first)
        assertEquals(markdown.indexOf("second") - 1, joined?.second)
    }

    @Test
    fun joinWithNextLineRemovesLineBreakAfterActiveLine() {
        val markdown = "first\nsecond\nthird"
        val second = MarkdownDocumentModel.activeRange(markdown, markdown.indexOf("second"))
        val joined = MarkdownDocumentModel.joinWithNextLine(markdown, second)

        assertEquals("first\nsecondthird", joined?.first)
        assertEquals(markdown.indexOf("third") - 1, joined?.second)
    }
}

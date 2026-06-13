package com.kloneets.kokotools

import android.content.Context
import android.graphics.Color
import android.graphics.Typeface
import android.view.inputmethod.InputMethodManager
import android.text.Editable
import android.text.Spannable
import android.text.SpannableString
import android.text.SpannableStringBuilder
import android.text.TextWatcher
import android.text.style.ForegroundColorSpan
import android.view.Gravity
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.ViewGroup
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import io.noties.markwon.Markwon
import io.noties.markwon.ext.tables.TablePlugin

data class MarkdownSegment(
    val rawStart: Int,
    val rawEnd: Int,
    val markdownText: String,
    val active: Boolean,
    val codeBlock: Boolean,
    val language: String = "",
    val displayRawStart: Int = rawStart,
    val renderAsSource: Boolean = false,
)

object MarkdownDocumentModel {
    fun segments(markdown: String, activeOffset: Int): List<MarkdownSegment> {
        if (markdown.isEmpty()) {
            return listOf(MarkdownSegment(0, 0, "", active = true, codeBlock = false))
        }
        val activeRange = activeRange(markdown, activeOffset)
        val segments = mutableListOf<MarkdownSegment>()
        val codeBlocks = MarkdownHighlighter.findCodeBlocks(markdown)
        var offset = 0
        while (offset < markdown.length) {
            val codeBlock = codeBlocks.firstOrNull { it.start == offset }
            val end = codeBlock?.end ?: lineEnd(markdown, offset)
            val isActive = offset == activeRange.first && end == activeRange.last + 1
            val raw = markdown.substring(offset, end)
            segments.add(
                MarkdownSegment(
                    rawStart = offset,
                    rawEnd = end,
                    markdownText = if (!isActive && codeBlock != null) {
                        markdown.substring(codeBlock.contentStart, codeBlock.contentEnd).trimEnd('\n', '\r')
                    } else if (!isActive) {
                        raw.trimEnd('\n', '\r')
                    } else {
                        raw
                    },
                    active = isActive,
                    codeBlock = codeBlock != null,
                    language = codeBlock?.language.orEmpty(),
                    displayRawStart = if (!isActive && codeBlock != null) codeBlock.contentStart else offset,
                ),
            )
            offset = end
        }
        if (activeRange.isEmpty() && activeRange.first == markdown.length) {
            segments.add(MarkdownSegment(markdown.length, markdown.length, "", active = true, codeBlock = false))
        }
        return protectListFragmentsAroundActiveLine(segments)
    }

    fun activeRange(markdown: String, activeOffset: Int): IntRange {
        if (markdown.isEmpty()) return 0 until 0
        val offset = activeOffset.coerceIn(0, markdown.length)
        MarkdownHighlighter.findCodeBlocks(markdown).firstOrNull { offset >= it.start && offset <= it.end }?.let {
            return it.start until it.end
        }
        val start = markdown.lastIndexOf('\n', (offset - 1).coerceAtLeast(0)).let { if (it < 0) 0 else it + 1 }
        val nextBreak = markdown.indexOf('\n', offset)
        val end = if (nextBreak < 0) markdown.length else nextBreak + 1
        return start until end
    }

    fun replaceRange(markdown: String, range: IntRange, replacement: String): String {
        return markdown.substring(0, range.first) + replacement + markdown.substring(range.last + 1)
    }

    fun editableText(segment: MarkdownSegment): String {
        if (segment.codeBlock) return segment.markdownText
        return segment.markdownText.removeSuffix("\r\n").removeSuffix("\n").removeSuffix("\r")
    }

    fun replacementText(segment: MarkdownSegment, editedText: String): String {
        if (segment.codeBlock) return editedText
        val lineBreak = when {
            segment.markdownText.endsWith("\r\n") -> "\r\n"
            segment.markdownText.endsWith("\n") -> "\n"
            segment.markdownText.endsWith("\r") -> "\r"
            else -> ""
        }
        if (lineBreak.isEmpty() || editedText.endsWith("\n") || editedText.endsWith("\r")) {
            return editedText
        }
        return editedText + lineBreak
    }

    fun joinWithPreviousLine(markdown: String, activeRange: IntRange): Pair<String, Int>? {
        if (activeRange.first <= 0 || activeRange.first > markdown.length) return null
        if (markdown[activeRange.first - 1] != '\n') return null
        val updated = markdown.removeRange(activeRange.first - 1, activeRange.first)
        return updated to (activeRange.first - 1)
    }

    fun joinWithNextLine(markdown: String, activeRange: IntRange): Pair<String, Int>? {
        val nextBreak = activeRange.last
        if (nextBreak < 0 || nextBreak >= markdown.length || markdown[nextBreak] != '\n') return null
        val updated = markdown.removeRange(nextBreak, nextBreak + 1)
        return updated to nextBreak
    }

    fun adjacentActiveOffset(markdown: String, activeRange: IntRange, direction: Int): Int {
        if (markdown.isEmpty()) return 0
        return if (direction > 0) {
            val next = (activeRange.last + 1).coerceAtMost(markdown.length)
            when {
                next >= markdown.length -> if (markdown.endsWith('\n') && activeRange.last == markdown.length - 1) {
                    markdown.length
                } else {
                    activeRange.first
                }
                markdown[next] == '\n' -> (next + 1).coerceAtMost(markdown.length)
                else -> next
            }
        } else {
            val previousEnd = (activeRange.first - 1).coerceAtLeast(0)
            if (activeRange.first <= 0) {
                activeRange.first
            } else {
                activeRange(markdown, previousEnd).first
            }
        }
    }

    fun rawOffsetForRenderedOffset(segment: MarkdownSegment, renderedOffset: Int): Int {
        val displayEnd = (segment.displayRawStart + segment.markdownText.length).coerceAtMost(segment.rawEnd)
        return (segment.displayRawStart + renderedOffset.coerceAtLeast(0))
            .coerceIn(segment.rawStart, displayEnd)
    }

    private fun lineEnd(markdown: String, start: Int): Int {
        val nextBreak = markdown.indexOf('\n', start)
        return if (nextBreak < 0) markdown.length else nextBreak + 1
    }

    private fun protectListFragmentsAroundActiveLine(segments: List<MarkdownSegment>): List<MarkdownSegment> {
        val activeIndex = segments.indexOfFirst { it.active }
        if (activeIndex < 0 || !isListBlock(segments[activeIndex].markdownText)) return segments
        return segments.mapIndexed { index, segment ->
            if (!segment.active && !segment.codeBlock && isListBlock(segment.markdownText) && kotlin.math.abs(index - activeIndex) == 1) {
                segment.copy(renderAsSource = true)
            } else {
                segment
            }
        }
    }

    private fun isListBlock(markdown: String): Boolean {
        val lines = markdown.lines().filter { it.isNotBlank() }
        return lines.isNotEmpty() && lines.all { LIST_LINE_REGEX.matches(it) }
    }

    private val LIST_LINE_REGEX = Regex("""^[ \t]*(?:[-+*]|\d+[.)])\s+(?:\[[ xX]\]\s+)?\S.*$""")
}

class HybridMarkdownEditor(
    context: Context,
    private val palette: AppPalette = AppPalette.light(),
    initialSpellCheckEnabled: Boolean = false,
    initialSpellDictionaries: List<String> = emptyList(),
) : ScrollView(context) {
    private val markwon = Markwon.builder(context)
        .usePlugin(TablePlugin.create(context))
        .build()
    private val highlighter = MarkdownHighlighter(palette.markdown)
    private val container = LinearLayout(context).apply {
        orientation = LinearLayout.VERTICAL
        layoutParams = LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT)
    }
    private var markdown = ""
    private var activeOffset = 0
    private var activeRange = 0 until 0
    private var keyboardVisible = false
    private var suppressChanges = false
    private var spellCheckEnabled = initialSpellCheckEnabled
    private var spellDictionaries = initialSpellDictionaries
    private var activeSegment: MarkdownSegment = MarkdownSegment(0, 0, "", active = true, codeBlock = false)
    private val activeEditText: EditText = createActiveEditor()
    private val spellChecker = AndroidSpellChecker(context, activeEditText)
    var onMarkdownChanged: ((String) -> Unit)? = null

    init {
        setBackgroundColor(Color.TRANSPARENT)
        isFillViewport = true
        clipToPadding = false
        setPadding(0, 0, 0, dp(16))
        addView(container)
        spellChecker.setConfig(spellCheckEnabled, spellDictionaries)
        setOnClickListener {
            focusEndOfNote()
        }
    }

    fun setMarkdown(text: String, activeOffset: Int = 0) {
        markdown = text
        this.activeOffset = activeOffset.coerceIn(0, markdown.length)
        rebuild()
    }

    fun updateMarkdownPreservingState(text: String) {
        val wasFocused = activeEditText.hasFocus()
        val nextOffset = if (wasFocused) {
            activeRange.first + activeEditText.selectionStart.coerceAtLeast(0)
        } else {
            activeOffset
        }
        val previousScrollY = scrollY
        markdown = text
        activeOffset = nextOffset.coerceIn(0, markdown.length)
        rebuild()
        if (wasFocused) {
            focusActiveEditor(showKeyboard = keyboardVisible)
        }
        post { scrollTo(0, previousScrollY) }
    }

    fun getMarkdown(): String = markdown

    fun setSpellCheckEnabled(enabled: Boolean) {
        spellCheckEnabled = enabled
        activeEditText.inputType = NoteEditorInputTypes.forSpellCheck(enabled)
        spellChecker.setConfig(enabled, spellDictionaries)
    }

    fun setSpellDictionaries(codes: List<String>) {
        spellDictionaries = codes
        spellChecker.setConfig(spellCheckEnabled, spellDictionaries)
    }

    fun scrollEditorToTop() {
        post { scrollTo(0, 0) }
    }

    private fun rebuild() {
        suppressChanges = true
        container.removeAllViews()
        val segments = MarkdownDocumentModel.segments(markdown, activeOffset)
        val nextActiveSegment = segments.firstOrNull { it.active } ?: MarkdownSegment(0, 0, "", active = true, codeBlock = false)
        bindActiveEditor(nextActiveSegment)
        for (segment in segments) {
            if (segment.active) {
                detachActiveEditor()
                container.addView(activeEditText)
            } else {
                container.addView(renderedSegment(segment))
            }
        }
        suppressChanges = false
    }

    private fun createActiveEditor(): EditText {
        return EditText(context).apply {
            id = R.id.note_editor
            textSize = 16f
            gravity = Gravity.TOP or Gravity.START
            inputType = NoteEditorInputTypes.forSpellCheck(spellCheckEnabled)
            setTextColor(palette.textPrimary)
            setHintTextColor(palette.textMuted)
            includeFontPadding = true
            setSingleLine(false)
            setPadding(dp(14), dp(2), dp(14), dp(2))
            background = null
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            )
            minHeight = dp(42)
            addTextChangedListener(MarkdownHighlightingTextWatcher(highlighter))
            addTextChangedListener(object : TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) = Unit
                override fun afterTextChanged(s: Editable?) {
                    if (suppressChanges) return
                    val replacement = MarkdownDocumentModel.replacementText(activeSegment, s?.toString().orEmpty())
                    markdown = MarkdownDocumentModel.replaceRange(markdown, activeRange, replacement)
                    activeRange = activeRange.first until (activeRange.first + replacement.length)
                    activeOffset = activeRange.first + selectionStart.coerceAtLeast(0)
                    onMarkdownChanged?.invoke(markdown)
                    spellChecker.onTextChanged()
                }
            })
            setOnKeyListener { _, keyCode, event ->
                if (event.action != KeyEvent.ACTION_DOWN) return@setOnKeyListener false
                when (keyCode) {
                    KeyEvent.KEYCODE_DPAD_DOWN -> moveActiveLine(direction = 1)
                    KeyEvent.KEYCODE_DPAD_UP -> moveActiveLine(direction = -1)
                    KeyEvent.KEYCODE_DEL -> handleBackspaceAtStart()
                    KeyEvent.KEYCODE_FORWARD_DEL -> handleDeleteAtEnd()
                    else -> false
                }
            }
            setOnFocusChangeListener { _, hasFocus ->
                if (hasFocus) {
                    keyboardVisible = true
                }
            }
            setOnTouchListener { _, event ->
                spellChecker.handleTouchEvent(event)
            }
        }
    }

    private fun bindActiveEditor(segment: MarkdownSegment) {
        activeSegment = segment
        activeRange = segment.rawStart until segment.rawEnd
        val minimumLines = if (segment.codeBlock) {
            MarkdownDocumentModel.editableText(segment).lines().size.coerceAtLeast(1)
        } else {
            1
        }
        activeEditText.minLines = minimumLines
        activeEditText.maxLines = Int.MAX_VALUE
        val nextText = MarkdownDocumentModel.editableText(segment)
        if (activeEditText.text.toString() != nextText) {
            activeEditText.setText(nextText)
        }
        val selection = (activeOffset - segment.rawStart).coerceIn(0, activeEditText.length())
        activeEditText.setSelection(selection)
    }

    private fun detachActiveEditor() {
        (activeEditText.parent as? ViewGroup)?.removeView(activeEditText)
    }

    private fun moveActiveLine(direction: Int): Boolean {
        val nextOffset = MarkdownDocumentModel.adjacentActiveOffset(markdown, activeRange, direction)
        if (nextOffset == activeRange.first) return false
        activeOffset = nextOffset
        rebuild()
        focusActiveEditor(showKeyboard = keyboardVisible)
        scrollActiveEditorIntoView()
        return true
    }

    private fun handleBackspaceAtStart(): Boolean {
        if (activeEditText.selectionStart != 0 || activeEditText.selectionEnd != 0) return false
        val (updated, nextOffset) = MarkdownDocumentModel.joinWithPreviousLine(markdown, activeRange) ?: return false
        markdown = updated
        activeOffset = nextOffset
        onMarkdownChanged?.invoke(markdown)
        rebuild()
        focusActiveEditor(showKeyboard = keyboardVisible)
        activeEditText.setSelection((nextOffset - activeSegment.rawStart).coerceAtLeast(0).coerceAtMost(activeEditText.length()))
        scrollActiveEditorIntoView()
        return true
    }

    private fun handleDeleteAtEnd(): Boolean {
        if (activeEditText.selectionStart != activeEditText.length() || activeEditText.selectionEnd != activeEditText.length()) return false
        val (updated, nextOffset) = MarkdownDocumentModel.joinWithNextLine(markdown, activeRange) ?: return false
        markdown = updated
        activeOffset = nextOffset
        onMarkdownChanged?.invoke(markdown)
        rebuild()
        focusActiveEditor(showKeyboard = keyboardVisible)
        activeEditText.setSelection(activeEditText.length())
        scrollActiveEditorIntoView()
        return true
    }

    private fun focusActiveEditor(showKeyboard: Boolean) {
        val editor = activeEditText
        editor.requestFocus()
        if (!showKeyboard) return
        editor.post {
            val inputMethodManager = context.getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager
            inputMethodManager?.showSoftInput(editor, InputMethodManager.SHOW_IMPLICIT)
        }
    }

    private fun focusEndOfNote() {
        activeOffset = markdown.length
        rebuild()
        activeEditText.setSelection(activeEditText.length())
        focusActiveEditor(showKeyboard = true)
        scrollActiveEditorIntoView()
    }

    private fun scrollActiveEditorIntoView() {
        activeEditText.post {
            val editorTop = activeEditText.top
            val editorBottom = activeEditText.bottom
            val visibleTop = scrollY
            val visibleBottom = scrollY + height - paddingBottom
            when {
                editorTop < visibleTop -> smoothScrollTo(0, editorTop)
                editorBottom > visibleBottom -> smoothScrollTo(0, editorBottom - (height - paddingBottom))
            }
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (event.action == MotionEvent.ACTION_UP && event.y + scrollY >= container.bottom) {
            focusEndOfNote()
            return true
        }
        return super.onTouchEvent(event)
    }

    override fun onDetachedFromWindow() {
        spellChecker.close()
        super.onDetachedFromWindow()
    }

    private fun renderedSegment(segment: MarkdownSegment): TextView {
        return TextView(context).apply {
            textSize = 16f
            setTextColor(palette.textPrimary)
            setPadding(dp(14), dp(2), dp(14), dp(2))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            )
            if (segment.codeBlock) {
                setTypeface(Typeface.MONOSPACE)
                setBackgroundColor(palette.markdown.codeBackground)
                setTextColor(palette.markdown.codeText)
                text = highlightedCode(segment.markdownText, segment.language)
            } else if (segment.renderAsSource) {
                text = highlightedMarkdownSource(segment.markdownText)
            } else {
                markwon.setMarkdown(this, segment.markdownText)
            }
            setOnTouchListener { view, event ->
                if (event.action != MotionEvent.ACTION_UP) return@setOnTouchListener false
                val textView = view as TextView
                val renderedOffset = textView.renderedOffsetForTouch(event)
                activeOffset = MarkdownDocumentModel.rawOffsetForRenderedOffset(segment, renderedOffset)
                rebuild()
                focusActiveEditor(showKeyboard = true)
                scrollActiveEditorIntoView()
                textView.performClick()
                true
            }
        }
    }

    private fun TextView.renderedOffsetForTouch(event: MotionEvent): Int {
        val layout = layout ?: return 0
        val x = event.x - totalPaddingLeft + scrollX
        val y = event.y - totalPaddingTop + scrollY
        val line = layout.getLineForVertical(y.toInt().coerceIn(0, layout.height))
        return layout.getOffsetForHorizontal(line, x.coerceAtLeast(0f))
    }

    private fun dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }

    private fun highlightedCode(code: String, language: String): SpannableString {
        val spannable = SpannableString(code)
        for (token in CodeSyntaxHighlighter.tokenize(language, code)) {
            val color = codeTokenColor(token.type) ?: continue
            if (token.start < token.end && token.end <= code.length) {
                spannable.setSpan(ForegroundColorSpan(color), token.start, token.end, Spannable.SPAN_EXCLUSIVE_EXCLUSIVE)
            }
        }
        return spannable
    }

    private fun highlightedMarkdownSource(markdown: String): SpannableStringBuilder {
        return SpannableStringBuilder(markdown).apply {
            highlighter.apply(this)
        }
    }

    private fun codeTokenColor(type: String?): Int? {
        return when (type) {
            "keyword", "important", "atrule", "selector" -> palette.markdown.codeKeyword
            "string", "char", "regex", "inserted" -> palette.markdown.codeString
            "number" -> palette.markdown.codeNumber
            "comment", "prolog", "doctype", "cdata" -> palette.markdown.codeComment
            "operator", "entity", "url" -> palette.markdown.codeOperator
            "punctuation" -> palette.markdown.codePunctuation
            "class-name", "builtin", "namespace" -> palette.markdown.codeType
            "property", "constant", "symbol" -> palette.markdown.codeProperty
            "function", "method" -> palette.markdown.codeFunction
            "boolean", "null", "nil", "deleted" -> palette.markdown.codeLiteral
            "tag" -> palette.markdown.codeTag
            "attr-name", "attr-value" -> palette.markdown.codeAttribute
            else -> null
        }
    }
}

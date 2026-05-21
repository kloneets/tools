package com.kloneets.kokotools

import android.content.Context
import android.graphics.Color
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.TextPaint
import android.text.style.CharacterStyle
import android.text.style.UpdateAppearance
import android.util.Log
import android.view.MotionEvent
import android.widget.EditText
import android.widget.PopupMenu
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException

class AndroidSpellChecker(
    context: Context,
    private val editor: EditText,
) {
    private val mainHandler = Handler(Looper.getMainLooper())
    private val repository = HunspellSpellEngineRepository(context.applicationContext, java.io.File(context.filesDir, "spell"))
    private var worker: ExecutorService = Executors.newSingleThreadExecutor()
    private var enabled = false
    private var dictionaryCodes: List<String> = emptyList()
    private var generation = 0
    private val pendingCheck = Runnable {
        runSpellCheck()
    }

    fun setConfig(nextEnabled: Boolean, nextDictionaryCodes: List<String>) {
        ensureWorker()
        enabled = nextEnabled
        dictionaryCodes = HunspellSpellEngine.normalizeCodes(nextDictionaryCodes)
        generation++
        mainHandler.removeCallbacks(pendingCheck)
        clearSpans()
        Log.w(TAG, "setConfig enabled=$enabled dictionaries=${dictionaryCodes.joinToString(",")}")
        if (enabled) {
            scheduleCheck()
        }
    }

    fun onTextChanged() {
        if (!enabled) return
        scheduleCheck()
    }

    fun close() {
        mainHandler.removeCallbacks(pendingCheck)
        enabled = false
        clearSpans()
        repository.close()
        worker.shutdownNow()
    }

    fun handleTouchEvent(event: MotionEvent): Boolean {
        if (!enabled || event.action != MotionEvent.ACTION_UP) return false
        val editable = editor.text ?: return false
        val offset = editor.getOffsetForPosition(event.x, event.y).coerceIn(0, editable.length)
        val span = editable.getSpans(offset, offset, AppSpellCheckUnderlineSpan::class.java).firstOrNull()
            ?: editable.getSpans((offset - 1).coerceAtLeast(0), offset, AppSpellCheckUnderlineSpan::class.java).firstOrNull()
            ?: return false
        val start = editable.getSpanStart(span).coerceAtLeast(0)
        val end = editable.getSpanEnd(span).coerceAtLeast(start)
        if (start >= end || end > editable.length) return false
        val word = editable.substring(start, end)
        requestSuggestions(word, SpellMisspelling(start, end))
        return true
    }

    private fun scheduleCheck() {
        mainHandler.removeCallbacks(pendingCheck)
        mainHandler.postDelayed(pendingCheck, CHECK_DELAY_MS)
    }

    private fun runSpellCheck() {
        if (!enabled) return
        val text = editor.text?.toString().orEmpty()
        if (text.isBlank()) {
            clearSpans()
            return
        }
        val requestGeneration = ++generation
        val codes = dictionaryCodes
        executeSpellWork {
            val dictionary = repository.load(codes)
            val misspellings = dictionary.misspellings(text)
            mainHandler.post {
                if (enabled && requestGeneration == generation) {
                    applyMisspellings(misspellings)
                    Log.w(TAG, "Local spellcheck dictionaries=${codes.joinToString(",")} typos=${misspellings.size}")
                }
            }
        }
    }

    private fun requestSuggestions(word: String, misspelling: SpellMisspelling) {
        val requestGeneration = generation
        val codes = dictionaryCodes
        executeSpellWork {
            val suggestions = repository.load(codes).suggestions(word)
            mainHandler.post {
                if (enabled && requestGeneration == generation) {
                    showSuggestions(misspelling, suggestions)
                }
            }
        }
    }

    private fun executeSpellWork(task: () -> Unit) {
        ensureWorker()
        try {
            worker.execute(task)
        } catch (_: RejectedExecutionException) {
            worker = Executors.newSingleThreadExecutor()
            worker.execute(task)
        }
    }

    private fun ensureWorker() {
        if (worker.isShutdown || worker.isTerminated) {
            worker = Executors.newSingleThreadExecutor()
        }
    }

    private fun showSuggestions(misspelling: SpellMisspelling, suggestions: List<String>) {
        val popup = PopupMenu(editor.context, editor)
        if (suggestions.isEmpty()) {
            popup.menu.add("No suggestions").isEnabled = false
        } else {
            suggestions.take(MAX_SUGGESTIONS).forEach { suggestion ->
                popup.menu.add(suggestion).setOnMenuItemClickListener {
                    replaceMisspelling(misspelling, suggestion)
                    true
                }
            }
        }
        popup.show()
    }

    private fun replaceMisspelling(misspelling: SpellMisspelling, suggestion: String) {
        val editable = editor.text ?: return
        val start = misspelling.start.coerceIn(0, editable.length)
        val end = misspelling.end.coerceIn(start, editable.length)
        editable.replace(start, end, suggestion)
        editor.setSelection((start + suggestion.length).coerceIn(0, editable.length))
    }

    private fun applyMisspellings(misspellings: List<SpellMisspelling>) {
        val editable = editor.text ?: return
        clearSpans(editable)
        misspellings.forEach { misspelling ->
            val start = misspelling.start.coerceIn(0, editable.length)
            val end = misspelling.end.coerceIn(start, editable.length)
            if (start < end) {
                editable.setSpan(AppSpellCheckUnderlineSpan(), start, end, Editable.SPAN_EXCLUSIVE_EXCLUSIVE)
            }
        }
    }

    private fun clearSpans(editable: Editable? = editor.text) {
        editable ?: return
        editable.getSpans(0, editable.length, AppSpellCheckUnderlineSpan::class.java).forEach { span ->
            editable.removeSpan(span)
        }
    }

    private class AppSpellCheckUnderlineSpan : CharacterStyle(), UpdateAppearance {
        override fun updateDrawState(textPaint: TextPaint) {
            textPaint.isUnderlineText = true
            textPaint.underlineColor = Color.RED
            textPaint.underlineThickness = UNDERLINE_THICKNESS
        }
    }

    companion object {
        private const val TAG = "KokoSpell"
        private const val CHECK_DELAY_MS = 450L
        private const val MAX_SUGGESTIONS = 8
        private const val UNDERLINE_THICKNESS = 3f
    }
}

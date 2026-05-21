package com.kloneets.kokotools

import android.text.InputType

object NoteEditorInputTypes {
    fun forSpellCheck(enabled: Boolean): Int {
        val base = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_MULTI_LINE
        return if (enabled) {
            base or InputType.TYPE_TEXT_FLAG_AUTO_CORRECT
        } else {
            base or InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        }
    }
}

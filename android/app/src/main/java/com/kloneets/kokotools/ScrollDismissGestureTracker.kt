package com.kloneets.kokotools

import android.view.MotionEvent
import kotlin.math.abs

class ScrollDismissGestureTracker(private val touchSlop: Float) {
    private var downX = 0f
    private var downY = 0f
    private var dismissedForGesture = false

    fun onTouch(action: Int, x: Float, y: Float): Boolean {
        return when (action) {
            MotionEvent.ACTION_DOWN -> {
                downX = x
                downY = y
                dismissedForGesture = false
                false
            }
            MotionEvent.ACTION_MOVE -> {
                if (!dismissedForGesture && abs(y - downY) > touchSlop && abs(y - downY) > abs(x - downX)) {
                    dismissedForGesture = true
                    true
                } else {
                    false
                }
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                dismissedForGesture = false
                false
            }
            else -> false
        }
    }
}

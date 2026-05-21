package com.kloneets.kokotools

import android.view.MotionEvent
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ScrollDismissGestureTrackerTest {
    @Test
    fun verticalDragPastSlopDismissesOnce() {
        val tracker = ScrollDismissGestureTracker(touchSlop = 8f)

        assertFalse(tracker.onTouch(MotionEvent.ACTION_DOWN, 10f, 20f))
        assertFalse(tracker.onTouch(MotionEvent.ACTION_MOVE, 11f, 25f))
        assertTrue(tracker.onTouch(MotionEvent.ACTION_MOVE, 12f, 40f))
        assertFalse(tracker.onTouch(MotionEvent.ACTION_MOVE, 12f, 60f))
    }

    @Test
    fun horizontalDragDoesNotDismissKeyboard() {
        val tracker = ScrollDismissGestureTracker(touchSlop = 8f)

        assertFalse(tracker.onTouch(MotionEvent.ACTION_DOWN, 10f, 20f))
        assertFalse(tracker.onTouch(MotionEvent.ACTION_MOVE, 30f, 22f))
    }
}

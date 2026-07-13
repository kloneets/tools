package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class RootPaddingTest {
    @Test
    fun withInsetsAddsInsetsToBasePadding() {
        val base = RootPadding(left = 2, top = 3, right = 5, bottom = 7)

        assertEquals(
            RootPadding(left = 13, top = 16, right = 22, bottom = 26),
            base.withInsets(left = 11, top = 13, right = 17, bottom = 19),
        )
    }

    @Test
    fun repeatedCalculationsDoNotAccumulatePreviousInsets() {
        val base = RootPadding(left = 2, top = 3, right = 5, bottom = 7)

        base.withInsets(left = 11, top = 13, right = 17, bottom = 19)

        assertEquals(
            RootPadding(left = 25, top = 32, right = 36, bottom = 38),
            base.withInsets(left = 23, top = 29, right = 31, bottom = 31),
        )
    }
}

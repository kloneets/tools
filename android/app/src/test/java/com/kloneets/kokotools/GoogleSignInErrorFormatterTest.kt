package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class GoogleSignInErrorFormatterTest {
    @Test
    fun `status 10 identifies developer configuration error`() {
        assertEquals(
            "Firebase Google login failed (status 10): developer configuration error",
            formatGoogleSignInError(10, null),
        )
    }

    @Test
    fun `status 12501 identifies cancellation`() {
        assertEquals(
            "Firebase Google login failed (status 12501): sign-in canceled",
            formatGoogleSignInError(12501, ""),
        )
    }

    @Test
    fun `unknown status preserves useful exception message`() {
        assertEquals(
            "Firebase Google login failed (status 99999): service unavailable",
            formatGoogleSignInError(99999, "service unavailable"),
        )
    }

    @Test
    fun `unknown status without message has fallback`() {
        assertEquals(
            "Firebase Google login failed (status 99999): unknown Google sign-in error",
            formatGoogleSignInError(99999, null),
        )
    }
}

package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test
import java.net.URLDecoder

class FirebaseSyncRepositoryTest {
    @Test
    fun googleSignInPostBodyUsesGoogleProviderAndIdToken() {
        val postBody = FirebaseSyncRepository.googleSignInPostBody("token with spaces")
        val values = postBody
            .split("&")
            .associate {
                val parts = it.split("=", limit = 2)
                parts[0] to URLDecoder.decode(parts[1], Charsets.UTF_8.name())
            }

        assertEquals("token with spaces", values["id_token"])
        assertEquals("google.com", values["providerId"])
    }
}

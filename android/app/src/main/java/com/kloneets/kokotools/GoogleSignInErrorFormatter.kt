package com.kloneets.kokotools

internal fun formatGoogleSignInError(statusCode: Int, message: String?): String {
    val detail = message?.takeIf { it.isNotBlank() } ?: when (statusCode) {
        10 -> "developer configuration error"
        12501 -> "sign-in canceled"
        else -> "unknown Google sign-in error"
    }
    return "Firebase Google login failed (status $statusCode): $detail"
}

package com.kloneets.kokotools

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

object SnapshotDateFormatter {
    private val outputFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")

    fun format(createdAt: String, zoneId: ZoneId = ZoneId.systemDefault()): String {
        if (createdAt.isBlank()) return ""
        return try {
            Instant.parse(createdAt).atZone(zoneId).format(outputFormatter)
        } catch (_: Exception) {
            createdAt
        }
    }
}

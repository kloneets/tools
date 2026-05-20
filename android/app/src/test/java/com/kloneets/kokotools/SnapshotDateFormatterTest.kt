package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.ZoneId

class SnapshotDateFormatterTest {
    @Test
    fun formatsIsoInstantInRequestedTimezone() {
        assertEquals(
            "2026-05-13 13:30:45",
            SnapshotDateFormatter.format("2026-05-13T10:30:45Z", ZoneId.of("Europe/Riga")),
        )
    }

    @Test
    fun keepsUnparseableValues() {
        assertEquals("snapshot-name", SnapshotDateFormatter.format("snapshot-name", ZoneId.of("UTC")))
    }
}

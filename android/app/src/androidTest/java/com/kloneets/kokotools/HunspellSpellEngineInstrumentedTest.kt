package com.kloneets.kokotools

import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class    HunspellSpellEngineInstrumentedTest {
    @Test
    fun loadsBundledDictionaries() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val repository = HunspellSpellEngineRepository(context, File(context.filesDir, "spell-test"))
        val engine = repository.load(SpellLanguages.supported.map { it.code })

        try {
            assertTrue(engine.ready())
            assertTrue(engine.misspellings("things friends answers is").isEmpty())
            assertFalse(engine.suggestions("wrnog").isEmpty())
        } finally {
            repository.close()
        }
    }
}

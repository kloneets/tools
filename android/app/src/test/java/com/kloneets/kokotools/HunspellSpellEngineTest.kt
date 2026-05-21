package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class HunspellSpellEngineTest {
    @Test
    fun tokenizesEnglishRussianLatvianApostrophesAndHyphens() {
        val tokens = HunspellSpellEngine.spellTokens("Hello, latviešu тест can't long-term x 123")

        assertEquals(
            listOf("Hello", "latviešu", "тест", "can't", "long-term"),
            tokens.map { it.word },
        )
    }

    @Test
    fun ignoresMarkdownCodeUrlsAndLinkUrls() {
        val text = """
            hello wrnog
            `inline_wrnog`
            ```
            block_wrnog
            ```
            https://example.com/wrnong
            [label](https://example.com/wrnong)
        """.trimIndent()
        val dictionary = HunspellSpellEngine(listOf(FakeDictionary(words = setOf("hello", "label"))))

        val misspellings = dictionary.misspellings(text)

        assertEquals(listOf("wrnog"), misspellings.map { text.substring(it.start, it.end) })
    }

    @Test
    fun checksAgainstAnyEnabledDictionary() {
        val dictionary = HunspellSpellEngine(
            listOf(
                FakeDictionary(words = setOf("hello", "world")),
                FakeDictionary(words = setOf("pasaule")),
            ),
        )

        val text = "hello pasaule wrong"
        val misspellings = dictionary.misspellings(text)

        assertEquals(listOf("wrong"), misspellings.map { text.substring(it.start, it.end) })
    }

    @Test
    fun suggestionsAreMergedDeduplicatedAndStable() {
        val dictionary = HunspellSpellEngine(
            listOf(
                FakeDictionary(suggestions = listOf("wrong", "wring")),
                FakeDictionary(suggestions = listOf("wrong", "wrote")),
            ),
        )

        assertEquals(listOf("wrong", "wring", "wrote"), dictionary.suggestions("wronk"))
    }

    @Test
    fun replacementUpdatesOnlySelectedWordRange() {
        val text = "hello wronk world"
        val updated = HunspellSpellEngine.replaceToken(text, SpellMisspelling(6, 11), "wrong")

        assertEquals("hello wrong world", updated)
    }

    @Test
    fun normalizesSupportedCodesAndDefaultsToEnglish() {
        assertEquals(listOf("en", "lv", "de"), HunspellSpellEngine.normalizeCodes(listOf(" EN ", "lv", "en", "de", "xx")))
        assertEquals(listOf("en"), HunspellSpellEngine.normalizeCodes(emptyList()))
    }

    @Test
    fun registryContainsExpectedLanguages() {
        assertEquals(
            listOf("en", "lv", "ru", "de", "fr", "es", "it", "pl", "lt", "et", "uk", "be"),
            SpellLanguages.supported.map { it.code },
        )
        assertEquals(
            listOf(
                "English",
                "Latvian",
                "Russian",
                "German",
                "French",
                "Spanish",
                "Italian",
                "Polish",
                "Lithuanian",
                "Estonian",
                "Ukrainian",
                "Belarusian",
            ),
            SpellLanguages.supported.map { it.label },
        )
    }

    private class FakeDictionary(
        private val words: Set<String> = emptySet(),
        private val suggestions: List<String> = emptyList(),
    ) : SpellDictionary {
        override fun spell(word: String): Boolean = word.lowercase() in words

        override fun suggest(word: String): List<String> = suggestions

        override fun close() = Unit
    }
}

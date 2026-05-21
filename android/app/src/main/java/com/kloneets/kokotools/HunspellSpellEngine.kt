package com.kloneets.kokotools

import android.content.Context
import android.util.Log
import java.io.File
import java.util.Locale
import java.util.concurrent.ConcurrentHashMap

data class SpellMisspelling(
    val start: Int,
    val end: Int,
)

data class SpellToken(
    val word: String,
    val start: Int,
    val end: Int,
)

data class SpellLanguage(
    val code: String,
    val label: String,
)

object SpellLanguages {
    val supported = listOf(
        SpellLanguage("en", "English"),
        SpellLanguage("lv", "Latvian"),
        SpellLanguage("ru", "Russian"),
        SpellLanguage("de", "German"),
        SpellLanguage("fr", "French"),
        SpellLanguage("es", "Spanish"),
        SpellLanguage("it", "Italian"),
        SpellLanguage("pl", "Polish"),
        SpellLanguage("lt", "Lithuanian"),
        SpellLanguage("et", "Estonian"),
        SpellLanguage("uk", "Ukrainian"),
        SpellLanguage("be", "Belarusian"),
    )

    private val supportedCodes = supported.map { it.code }.toSet()

    fun normalizeCodes(codes: List<String>): List<String> {
        val normalized = codes
            .map { it.trim().lowercase(Locale.ROOT) }
            .filter { it in supportedCodes }
            .distinct()
        return normalized.ifEmpty { listOf("en") }
    }
}

interface SpellDictionary : AutoCloseable {
    fun spell(word: String): Boolean
    fun suggest(word: String): List<String>
}

class HunspellNative {
    external fun open(affPath: String, dicPath: String): Long
    external fun spell(handle: Long, word: String): Boolean
    external fun suggest(handle: Long, word: String): Array<String>
    external fun close(handle: Long)

    companion object {
        init {
            System.loadLibrary("koko_spell")
        }
    }
}

class HunspellDictionary private constructor(
    private val native: HunspellNative,
    private val handle: Long,
) : SpellDictionary {
    override fun spell(word: String): Boolean = native.spell(handle, word)

    override fun suggest(word: String): List<String> = native.suggest(handle, word).toList()

    override fun close() {
        native.close(handle)
    }

    companion object {
        fun open(affFile: File, dicFile: File, native: HunspellNative = HunspellNative()): HunspellDictionary? {
            val handle = native.open(affFile.absolutePath, dicFile.absolutePath)
            return if (handle == 0L) null else HunspellDictionary(native, handle)
        }
    }
}

class HunspellSpellEngineRepository(
    private val context: Context,
    private val rootDir: File,
) {
    private val cache = ConcurrentHashMap<String, SpellDictionary>()
    private val failed = ConcurrentHashMap.newKeySet<String>()

    fun load(codes: List<String>): HunspellSpellEngine {
        val dictionaries = HunspellSpellEngine.normalizeCodes(codes).mapNotNull { code ->
            loadDictionary(code)
        }
        return HunspellSpellEngine(dictionaries)
    }

    fun close() {
        cache.values.forEach { dictionary ->
            runCatching { dictionary.close() }
        }
        cache.clear()
    }

    private fun loadDictionary(code: String): SpellDictionary? {
        cache[code]?.let { return it }
        if (failed.contains(code)) return null

        val affFile = dictionaryFile(code, "aff")
        val dicFile = dictionaryFile(code, "dic")
        if (!copyBundledDictionary(code, "aff", affFile) || !copyBundledDictionary(code, "dic", dicFile)) {
            failed.add(code)
            return null
        }

        return HunspellDictionary.open(affFile, dicFile)
            ?.also {
                cache[code] = it
                Log.w(TAG, "Loaded Hunspell dictionary: $code")
            }
            ?: run {
                failed.add(code)
                Log.w(TAG, "Hunspell dictionary failed to load: $code")
                null
            }
    }

    private fun copyBundledDictionary(code: String, extension: String, destination: File): Boolean {
        if (destination.isFile && destination.length() > 0) return true
        return runCatching {
            rootDir.mkdirs()
            context.assets.open("spell/$code.$extension").use { input ->
                destination.outputStream().use { output ->
                    input.copyTo(output)
                }
            }
            true
        }.getOrElse {
            Log.w(TAG, "Bundled spell dictionary copy failed for $code.$extension: ${it.message}")
            false
        }
    }

    private fun dictionaryFile(code: String, extension: String): File = File(rootDir, "$code.$extension")

    companion object {
        private const val TAG = "KokoSpell"
    }
}

class HunspellSpellEngine(private val dictionaries: List<SpellDictionary>) {
    fun ready(): Boolean = dictionaries.isNotEmpty()

    fun misspellings(text: String): List<SpellMisspelling> {
        if (dictionaries.isEmpty()) return emptyList()
        return spellTokens(text)
            .filterNot { token -> dictionaries.any { dictionary -> dictionary.spell(token.word) } }
            .map { token -> SpellMisspelling(token.start, token.end) }
    }

    fun suggestions(word: String): List<String> {
        val seen = LinkedHashSet<String>()
        dictionaries.forEach { dictionary ->
            dictionary.suggest(word).forEach { suggestion ->
                if (suggestion.isNotBlank()) {
                    seen.add(suggestion)
                }
            }
        }
        return seen.toList()
    }

    companion object {
        fun normalizeCodes(codes: List<String>): List<String> = SpellLanguages.normalizeCodes(codes)

        fun spellTokens(text: String): List<SpellToken> {
            val ignored = ignoredMarkdownRanges(text)
            val tokens = mutableListOf<SpellToken>()
            var index = 0
            while (index < text.length) {
                if (ignored.any { index in it }) {
                    index = ignored.first { index in it }.last + 1
                    continue
                }
                val codePoint = text.codePointAt(index)
                val charCount = Character.charCount(codePoint)
                if (!isSpellWordCodePoint(codePoint)) {
                    index += charCount
                    continue
                }
                val start = index
                index += charCount
                while (index < text.length) {
                    if (ignored.any { index in it }) break
                    val next = text.codePointAt(index)
                    if (!isSpellWordCodePoint(next)) break
                    index += Character.charCount(next)
                }
                tokenFrom(text, start, index)?.let { tokens.add(it) }
            }
            return tokens
        }

        fun ignoredMarkdownRanges(text: String): List<IntRange> {
            val ranges = mutableListOf<IntRange>()
            MarkdownHighlighter.findCodeBlocks(text).forEach { block ->
                ranges.add(block.start until block.end)
            }
            INLINE_CODE_REGEX.findAll(text).forEach { match ->
                ranges.add(match.range)
            }
            URL_REGEX.findAll(text).forEach { match ->
                ranges.add(match.range)
            }
            MARKDOWN_LINK_URL_REGEX.findAll(text).forEach { match ->
                match.groups[1]?.range?.let { ranges.add(it) }
            }
            return ranges.sortedBy { it.first }.mergeRanges()
        }

        fun replaceToken(text: String, token: SpellMisspelling, replacement: String): String {
            val start = token.start.coerceIn(0, text.length)
            val end = token.end.coerceIn(start, text.length)
            return text.substring(0, start) + replacement + text.substring(end)
        }

        private fun tokenFrom(text: String, start: Int, end: Int): SpellToken? {
            var tokenStart = start
            var tokenEnd = end
            while (tokenStart < tokenEnd && isTrimmedWordEdge(text[tokenStart])) tokenStart++
            while (tokenEnd > tokenStart && isTrimmedWordEdge(text[tokenEnd - 1])) tokenEnd--
            if (tokenStart >= tokenEnd) return null
            val word = text.substring(tokenStart, tokenEnd)
            return if (shouldCheckSpellWord(word)) SpellToken(word, tokenStart, tokenEnd) else null
        }

        private fun isSpellWordCodePoint(codePoint: Int): Boolean {
            return Character.isLetter(codePoint) || codePoint == '\''.code || codePoint == '’'.code || codePoint == '-'.code
        }

        private fun shouldCheckSpellWord(word: String): Boolean {
            if (word.codePointCount(0, word.length) <= 1) return false
            var hasLetter = false
            var index = 0
            while (index < word.length) {
                val codePoint = word.codePointAt(index)
                if (Character.isDigit(codePoint)) return false
                if (Character.isLetter(codePoint)) hasLetter = true
                index += Character.charCount(codePoint)
            }
            return hasLetter
        }

        private fun isTrimmedWordEdge(char: Char): Boolean = char == '\'' || char == '’' || char == '-'

        private fun List<IntRange>.mergeRanges(): List<IntRange> {
            if (isEmpty()) return emptyList()
            val merged = mutableListOf<IntRange>()
            for (range in this) {
                if (range.isEmpty()) continue
                val previous = merged.lastOrNull()
                if (previous == null || range.first > previous.last + 1) {
                    merged.add(range)
                } else {
                    merged[merged.lastIndex] = previous.first..maxOf(previous.last, range.last)
                }
            }
            return merged
        }

        private val INLINE_CODE_REGEX = Regex("""`[^`\n]+`""")
        private val URL_REGEX = Regex("""https?://[^\s)]+""")
        private val MARKDOWN_LINK_URL_REGEX = Regex("""\[[^\]]*]\(([^)\s]+)(?:\s+"[^"]*")?\)""")
    }
}

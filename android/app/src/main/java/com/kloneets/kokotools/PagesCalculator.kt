package com.kloneets.kokotools

class PagesCalculator {
    fun calculate(readText: String, firstText: String, secondText: String): PagesResult {
        val readPages = readText.toIntOrNull() ?: 1
        val firstBookPages = firstText.toIntOrNull().takeUnless { it == null || it == 0 } ?: 1
        val secondBookPages = secondText.toIntOrNull() ?: 1
        return calculate(readPages, firstBookPages, secondBookPages)
    }

    fun calculate(readPages: Int, firstBookPages: Int, secondBookPages: Int): PagesResult {
        val safeFirstBookPages = if (firstBookPages == 0) 1 else firstBookPages
        val convertedPages = readPages * secondBookPages / safeFirstBookPages
        val percent = readPages * 100 / safeFirstBookPages
        return PagesResult(
            readPages = readPages,
            firstBookPages = safeFirstBookPages,
            secondBookPages = secondBookPages,
            convertedPages = convertedPages,
            percent = percent,
        )
    }
}

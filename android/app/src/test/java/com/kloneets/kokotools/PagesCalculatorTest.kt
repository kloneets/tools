package com.kloneets.kokotools

import org.junit.Assert.assertEquals
import org.junit.Test

class PagesCalculatorTest {
    private val calculator = PagesCalculator()

    @Test
    fun calculateUsesDesktopFormula() {
        val result = calculator.calculate(readPages = 25, firstBookPages = 100, secondBookPages = 320)

        assertEquals(80, result.convertedPages)
        assertEquals(25, result.percent)
        assertEquals("80 pages, 25%", result.label)
    }

    @Test
    fun calculateSanitizesInvalidValuesLikeDesktop() {
        val result = calculator.calculate(readText = "x", firstText = "0", secondText = "y")

        assertEquals(1, result.readPages)
        assertEquals(1, result.firstBookPages)
        assertEquals(1, result.secondBookPages)
        assertEquals(1, result.convertedPages)
        assertEquals(100, result.percent)
    }
}

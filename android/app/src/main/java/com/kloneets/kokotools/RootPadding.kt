package com.kloneets.kokotools

data class RootPadding(
    val left: Int,
    val top: Int,
    val right: Int,
    val bottom: Int,
) {
    fun withInsets(left: Int, top: Int, right: Int, bottom: Int): RootPadding {
        return RootPadding(
            left = this.left + left,
            top = this.top + top,
            right = this.right + right,
            bottom = this.bottom + bottom,
        )
    }
}

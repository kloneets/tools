package com.kloneets.kokotools

import android.widget.EditText
import android.widget.TextView
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MainActivityTest {
    @Test
    fun opensNotesScreen() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                assertTrue(activity.findViewById<EditText>(R.id.note_editor).isShown)
            }
        }
    }

    @Test
    fun editingAndSavingNotePersistsContent() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                activity.findViewById<EditText>(R.id.note_editor).setText("hello")
                MainActivity::class.java.getDeclaredMethod("saveCurrentNote").apply {
                    isAccessible = true
                    invoke(activity)
                }

                val repository = NotesRepository(activity)
                assertEquals("hello", repository.read("untitled.md"))
            }
        }
    }

    @Test
    fun pagesScreenRecalculatesAfterInput() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                MainActivity::class.java.getDeclaredMethod("showPages").apply {
                    isAccessible = true
                    invoke(activity)
                }
                activity.findViewById<EditText>(R.id.pages_first).setText("100")
                activity.findViewById<EditText>(R.id.pages_read).setText("25")
                activity.findViewById<EditText>(R.id.pages_second).setText("320")

                assertEquals("80 pages, 25%", activity.findViewById<TextView>(R.id.pages_result).text.toString())
            }
        }
    }
}

package com.kloneets.kokotools

import android.view.View
import android.view.ViewGroup
import android.widget.EditText
import android.widget.ImageButton
import android.widget.RadioButton
import android.widget.TextView
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
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
                assertNull(activity.findViewById<TextView>(R.id.note_selector))
                assertTrue(activity.findViewById<ImageButton>(R.id.tools_menu).isShown)
                assertTrue(activity.findViewById<ImageButton>(R.id.actions_menu).isShown)
                assertNull(activity.findViewById<View>(R.id.notes_list))
            }
        }
    }

    @Test
    fun hamburgerDrawerNavigatesBetweenTools() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                activity.findViewById<ImageButton>(R.id.tools_menu).performClick()

                assertEquals(View.VISIBLE, activity.findViewById<View>(R.id.drawer_scrim).visibility)
                assertEquals(View.VISIBLE, activity.findViewById<View>(R.id.drawer_panel).visibility)

                activity.findViewById<TextView>(R.id.tab_pages).performClick()

                assertEquals("Pages", activity.findViewById<TextView>(R.id.app_title).text.toString())
                assertTrue(activity.findViewById<EditText>(R.id.pages_first).isShown)
            }
        }
    }

    @Test
    fun actionIconOpensCurrentActionMenu() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val actions = activity.findViewById<ImageButton>(R.id.actions_menu)

                assertTrue(actions.isShown)
                actions.performClick()
            }
        }
    }

    @Test
    fun editingNoteAutosavesRawMarkdownContent() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                NotesRepository(activity).clearAll()
                setPrivateField(activity, "currentNotePath", "")
                setPrivateField(activity, "settings", AppSettings())
                MainActivity::class.java.getDeclaredMethod("showNotes").apply {
                    isAccessible = true
                    invoke(activity)
                }
                val markdown = "# Title\n\nUse **bold**, `code`, and [link](https://example.com)."
                activity.findViewById<EditText>(R.id.note_editor).setText(markdown)
            }
            Thread.sleep(900)
            scenario.onActivity { activity ->
                val markdown = "# Title\n\nUse **bold**, `code`, and [link](https://example.com)."
                assertEquals(markdown, NotesRepository(activity).read("untitled.md"))
            }
        }
    }

    @Test
    fun notePickerSelectsNote() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val repository = NotesRepository(activity)
                repository.clearAll()
                repository.save("first.md", "one")
                repository.save("folder/second.md", "two")
                setPrivateField(activity, "currentNotePath", "")
                setPrivateField(activity, "settings", AppSettings())
                MainActivity::class.java.getDeclaredMethod("showNotes").apply {
                    isAccessible = true
                    invoke(activity)
                }

                MainActivity::class.java.getDeclaredMethod("showNotePicker").apply {
                    isAccessible = true
                    invoke(activity)
                }

                val picker = activity.findViewById<android.widget.ListView>(R.id.note_picker_list)
                assertTrue(picker.isShown)
                picker.performItemClick(
                    picker.adapter.getView(1, null, picker),
                    1,
                    picker.adapter.getItemId(1),
                )

                assertEquals("two", activity.findViewById<EditText>(R.id.note_editor).text.toString())
                assertEquals("folder/second.md", getPrivateField(activity, "currentNotePath"))
            }
        }
    }

    @Test
    fun cleanFocusedRawNoteAppliesRemoteTextWithoutLosingFocusOrSelection() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val repository = NotesRepository(activity)
                repository.clearAll()
                repository.save("a.md", "alpha\nbeta")
                setPrivateField(activity, "currentNotePath", "a.md")
                setPrivateField(
                    activity,
                    "settings",
                    AppSettings(notesApp = NotesSettings(currentNotePath = "a.md", previewHidden = true)),
                )
                showNotes(activity)

                val editor = activity.findViewById<EditText>(R.id.note_editor)
                editor.requestFocus()
                editor.setSelection(4)

                applyRemoteNotes(
                    activity,
                    listOf(FirebaseRemoteNote("a", "a.md", "alphabet\nbeta", 1L, deleted = false)),
                )

                assertEquals("alphabet\nbeta", editor.text.toString())
                assertEquals("a.md", getPrivateField(activity, "currentNotePath"))
                assertTrue(editor.hasFocus())
                assertEquals(4, editor.selectionStart)
            }
        }
    }

    @Test
    fun cleanFocusedRichNoteKeepsEditorSurfaceWhenRemoteTextChanges() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val repository = NotesRepository(activity)
                repository.clearAll()
                repository.save("rich.md", "alpha\n\n**beta**")
                setPrivateField(activity, "currentNotePath", "rich.md")
                setPrivateField(
                    activity,
                    "settings",
                    AppSettings(notesApp = NotesSettings(currentNotePath = "rich.md", previewHidden = false)),
                )
                showNotes(activity)

                val hybridBefore = getPrivateField(activity, "noteEditor")
                val editor = activity.findViewById<EditText>(R.id.note_editor)
                editor.requestFocus()
                editor.setSelection(3)

                applyRemoteNotes(
                    activity,
                    listOf(FirebaseRemoteNote("rich", "rich.md", "alphabet\n\n**beta**", 1L, deleted = false)),
                )

                assertEquals(hybridBefore, getPrivateField(activity, "noteEditor"))
                assertEquals("rich.md", getPrivateField(activity, "currentNotePath"))
                assertTrue(editor.hasFocus())
                assertEquals(3, editor.selectionStart)
            }
        }
    }

    @Test
    fun dirtyFocusedNoteDefersRemoteTextAndKeepsTypedContentVisible() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val repository = NotesRepository(activity)
                repository.clearAll()
                repository.save("dirty.md", "local")
                setPrivateField(activity, "currentNotePath", "dirty.md")
                setPrivateField(
                    activity,
                    "settings",
                    AppSettings(notesApp = NotesSettings(currentNotePath = "dirty.md", previewHidden = true)),
                )
                showNotes(activity)

                val editor = activity.findViewById<EditText>(R.id.note_editor)
                editor.requestFocus()
                editor.setText("local draft")

                applyRemoteNotes(
                    activity,
                    listOf(FirebaseRemoteNote("dirty", "dirty.md", "remote", 1L, deleted = false)),
                )

                assertEquals("local draft", editor.text.toString())
                assertEquals("local", repository.read("dirty.md"))
                val pending = getPrivateField(activity, "pendingRemoteNotes") as Map<*, *>
                assertTrue(pending.containsKey("dirty.md"))
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

    @Test
    fun syncScreenShowsFirebaseActionsOnly() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                setPrivateField(activity, "settings", AppSettings())
                showSync(activity)

                assertEquals("Sync", activity.findViewById<TextView>(R.id.app_title).text.toString())
                val labels = visibleTextLabels(activity.findViewById(android.R.id.content))
                assertTrue(labels.contains("Sync to Firebase"))
                assertTrue(!labels.contains("Pull todos now"))
                assertTrue(!labels.contains("Push todos now"))
                assertTrue(!labels.contains("Pull settings and assets now"))
                assertTrue(!labels.contains("Push settings and assets now"))
                assertTrue(!labels.contains("Replace local from Firebase"))
            }
        }
    }

    @Test
    fun settingsScreenChangesThemeMode() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                setPrivateField(activity, "settings", AppSettings(androidApp = AndroidSettings(themeMode = ThemeMode.System)))
                MainActivity::class.java.getDeclaredMethod("showSettings").apply {
                    isAccessible = true
                    invoke(activity)
                }

                activity.findViewById<RadioButton>(R.id.settings_theme_dark).performClick()

                val settings = getPrivateField(activity, "settings") as AppSettings
                assertEquals(ThemeMode.Dark, settings.androidApp.themeMode)
                assertTrue(activity.findViewById<RadioButton>(R.id.settings_theme_dark).isChecked)
                assertTrue(activity.findViewById<TextView>(R.id.settings_privacy_policy).isShown)
                assertTrue(activity.findViewById<TextView>(R.id.settings_delete_account).isShown)
            }
        }
    }

    private fun showSync(activity: MainActivity) {
        MainActivity::class.java.getDeclaredMethod("showSync").apply {
            isAccessible = true
            invoke(activity)
        }
    }

    private fun showNotes(activity: MainActivity) {
        MainActivity::class.java.getDeclaredMethod("showNotes").apply {
            isAccessible = true
            invoke(activity)
        }
    }

    private fun applyRemoteNotes(activity: MainActivity, notes: List<FirebaseRemoteNote>) {
        MainActivity::class.java.getDeclaredMethod("applyRemoteNotes", List::class.java).apply {
            isAccessible = true
            invoke(activity, notes)
        }
    }

    private fun setPrivateField(activity: MainActivity, name: String, value: Any?) {
        MainActivity::class.java.getDeclaredField(name).apply {
            isAccessible = true
            set(activity, value)
        }
    }

    private fun getPrivateField(activity: MainActivity, name: String): Any? {
        return MainActivity::class.java.getDeclaredField(name).run {
            isAccessible = true
            get(activity)
        }
    }

    private fun visibleTextLabels(view: View): Set<String> {
        val labels = mutableSetOf<String>()
        if (view is TextView && view.isShown) {
            labels += view.text.toString()
        }
        if (view is ViewGroup) {
            for (index in 0 until view.childCount) {
                labels += visibleTextLabels(view.getChildAt(index))
            }
        }
        return labels
    }
}

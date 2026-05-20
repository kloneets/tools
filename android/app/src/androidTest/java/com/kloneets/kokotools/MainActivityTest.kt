package com.kloneets.kokotools

import android.view.View
import android.widget.Button
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
    fun syncScreenShowsActionsForConnectionAndFolderState() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                setPrivateField(activity, "driveAccessToken", null)
                setPrivateField(activity, "settings", AppSettings())
                showSync(activity)

                assertEquals(View.VISIBLE, activity.findViewById<Button>(R.id.sync_connect).visibility)
                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_select_folder).visibility)
                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_upload).visibility)
                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_refresh).visibility)

                setPrivateField(activity, "driveAccessToken", "token")
                setPrivateField(activity, "settings", AppSettings())
                showSync(activity)

                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_connect).visibility)
                assertEquals(View.VISIBLE, activity.findViewById<Button>(R.id.sync_select_folder).visibility)
                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_upload).visibility)
                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_refresh).visibility)

                setPrivateField(
                    activity,
                    "settings",
                    AppSettings(
                        gdrive = GDriveSettings(
                            folderId = "folder",
                            folderName = "Koko Tools",
                            selectedSnapshotId = "snapshot",
                            snapshots = listOf(DriveSnapshotMeta("snapshot", "snapshot", "2026-05-13T10:00:00Z")),
                        ),
                    ),
                )
                setPrivateField(activity, "selectedSnapshotId", "snapshot")
                showSync(activity)

                assertEquals(View.GONE, activity.findViewById<Button>(R.id.sync_connect).visibility)
                assertEquals(View.VISIBLE, activity.findViewById<Button>(R.id.sync_select_folder).visibility)
                assertEquals(View.VISIBLE, activity.findViewById<Button>(R.id.sync_upload).visibility)
                assertEquals(View.VISIBLE, activity.findViewById<Button>(R.id.sync_refresh).visibility)
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
            }
        }
    }

    private fun showSync(activity: MainActivity) {
        MainActivity::class.java.getDeclaredMethod("showSync").apply {
            isAccessible = true
            invoke(activity)
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
}

package com.kloneets.kokotools

import android.app.Activity
import android.app.AlertDialog
import android.content.res.ColorStateList
import android.content.res.Configuration
import android.content.Intent
import android.net.Uri
import android.graphics.Paint
import android.graphics.Rect
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.InputType
import android.text.TextWatcher
import android.util.Log
import android.util.TypedValue
import android.view.Gravity
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import android.view.ViewGroup
import android.view.inputmethod.InputMethodManager
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.PopupMenu
import android.widget.RadioButton
import android.widget.RadioGroup
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import com.google.android.gms.auth.api.signin.GoogleSignIn
import com.google.android.gms.auth.api.signin.GoogleSignInOptions
import com.google.android.gms.common.api.ApiException
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

class MainActivity : Activity() {
    private lateinit var settingsRepository: SettingsRepository
    private lateinit var notesRepository: NotesRepository
    private lateinit var todoRepository: TodoRepository
    private lateinit var firebaseSyncRepository: FirebaseSyncRepository
    private val pagesCalculator = PagesCalculator()

    private var settings = AppSettings()
    private var todoStore = TodoStore()
    private var currentNotePath = ""
    private var noteList: List<NoteFile> = emptyList()
    private var firebaseSession: FirebaseSession? = null
    private var currentScreen = Screen.Todo
    private var palette = AppPalette.light()

    private lateinit var root: FrameLayout
    private lateinit var appLayout: LinearLayout
    private lateinit var titleText: TextView
    private lateinit var subtitleText: TextView
    private lateinit var actionsButton: ImageButton
    private lateinit var syncToolbarControl: FrameLayout
    private lateinit var syncProblemBadge: ImageView
    private lateinit var drawerScrim: View
    private lateinit var drawerPanel: LinearLayout
    private lateinit var content: LinearLayout

    private var noteListView: ListView? = null
    private var noteSelector: TextView? = null
    private var noteEditor: HybridMarkdownEditor? = null
    private var rawNoteEditor: EditText? = null
    private var rawNoteSpellChecker: AndroidSpellChecker? = null
    private var loadedNoteText = ""
    private val noteAutosaveHandler = Handler(Looper.getMainLooper())
    private var pendingNoteAutosave: Runnable? = null
    private var suppressNoteAutosave = false
    private val notePickerExpandedFolders = mutableSetOf<String>()

    private var firstInput: EditText? = null
    private var readInput: EditText? = null
    private var secondInput: EditText? = null
    private var resultText: TextView? = null
    private var rawNoteTouchDownAt = 0L

    private var syncStatusText: TextView? = null
    private var todoRefreshRunnable: Runnable? = null
    private var firebasePullRunnable: Runnable? = null
    private var firebaseSettingsPushRunnable: Runnable? = null
    private val firebaseViewSyncLock = Any()
    private val firebaseViewSyncInFlight = mutableSetOf<FirebaseViewSyncScope>()
    private var pagesLocalEditUntilMs = 0L
    private var pagesLocalEditAtMs = 0L
	private var todoDraftText = ""
	private var lastSyncStatus = ""
    private val pendingRemoteNotes = mutableMapOf<String, FirebaseRemoteNote>()
    private var todoMoveMode = false
    private val todoMoveRows = mutableMapOf<String, View>()
    private var todoDraggingId: String? = null
    private val expandedTodoArchiveMonths = mutableSetOf<String>()
    private val loadingTodoArchiveMonths = mutableSetOf<String>()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.enableEdgeToEdge(window)
        settingsRepository = SettingsRepository(this)
        notesRepository = NotesRepository(this)
        todoRepository = TodoRepository(this)
        firebaseSyncRepository = FirebaseSyncRepository(this)
        notesRepository.cleanupManagedAssetDirs()
        settings = settingsRepository.load()
        todoStore = todoRepository.load()
        palette = resolvePalette()
        applySystemBars()
        currentNotePath = settings.notesApp.currentNotePath
        buildRoot()
        showLastScreen()
        startFirebaseRealtimeIfEnabled()
        syncCurrentScreenToFirebase(force = true)
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == FIREBASE_GOOGLE_SIGN_IN_REQUEST_CODE) {
            handleFirebaseGoogleSignInResult(resultCode, data)
            return
        }
    }

    private fun buildRoot() {
        root = FrameLayout(this).apply {
            setBackgroundColor(COLOR_APP_BACKGROUND)
        }
        appLayout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(COLOR_APP_BACKGROUND)
            layoutParams = FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT,
            )
        }

        val toolbar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            minimumHeight = dp(68)
            setPadding(dp(16), dp(18), dp(16), dp(14))
            setBackgroundColor(COLOR_TOOLBAR)
        }

        val toolsButton = toolbarIconButton(
            icon = R.drawable.ic_menu_24,
            id = R.id.tools_menu,
            description = "Open navigation",
        ) {
            showNavigationDrawer()
        }
        val titleGroup = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
                .apply { leftMargin = dp(10) }
        }
        titleText = TextView(this).apply {
            id = R.id.app_title
            text = "Notes"
            textSize = 22f
            setTypeface(Typeface.DEFAULT, Typeface.BOLD)
            setTextColor(COLOR_TEXT_PRIMARY)
        }
        subtitleText = TextView(this).apply {
            text = "Markdown workspace"
            textSize = 13f
            setTextColor(COLOR_TEXT_SECONDARY)
        }
        titleGroup.addView(titleText)
        titleGroup.addView(subtitleText)

        actionsButton = toolbarIconButton(
            icon = R.drawable.ic_more_vert_24,
            id = R.id.actions_menu,
            description = "Open actions",
        ) {
            showCurrentActionsMenu()
        }
        val syncButton = toolbarIconButton(
            icon = R.drawable.ic_sync_24,
            id = R.id.sync_toolbar,
            description = "Quick sync to Firebase",
        ) {
            if (firebaseHasProblem()) showSync() else syncToFirebase()
        }.apply {
            contentDescription = null
            setOnClickListener(null)
            isFocusable = false
            layoutParams = FrameLayout.LayoutParams(dp(48), dp(48))
        }
        syncProblemBadge = ImageView(this).apply {
            id = R.id.sync_problem_badge
            setImageResource(R.drawable.ic_warning_18)
            contentDescription = "Firebase sync needs attention"
            visibility = View.GONE
            layoutParams = FrameLayout.LayoutParams(dp(18), dp(18), Gravity.TOP or Gravity.END).apply {
                topMargin = dp(2)
                marginEnd = dp(2)
            }
        }
        syncToolbarControl = FrameLayout(this).apply {
            contentDescription = "Quick sync to Firebase"
            isClickable = true
            isFocusable = true
            background = selectableBorderlessBackground()
            setOnClickListener {
                if (firebaseHasProblem()) showSync() else syncToFirebase()
            }
            layoutParams = LinearLayout.LayoutParams(dp(48), dp(48)).apply { leftMargin = dp(4) }
            addView(syncButton)
            addView(syncProblemBadge)
        }
        toolbar.addView(toolsButton)
        toolbar.addView(titleGroup)
        toolbar.addView(syncToolbarControl)
        toolbar.addView(actionsButton)

        content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), dp(16), dp(16), dp(16))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                1f,
            )
        }

        appLayout.addView(toolbar)
        appLayout.addView(content)
        root.addView(appLayout)
        buildNavigationDrawer()
        setContentView(root)
        applyWindowInsets()
        ViewCompat.requestApplyInsets(root)
        updateSyncProblemIndicator()
    }

    private fun applyWindowInsets() {
        val appBasePadding = appLayout.rootPadding()
        val drawerBasePadding = drawerPanel.rootPadding()
        ViewCompat.setOnApplyWindowInsetsListener(root) { _, windowInsets ->
            val insets = windowInsets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout(),
            )
            appLayout.applyPadding(appBasePadding.withInsets(insets.left, insets.top, insets.right, insets.bottom))
            drawerPanel.applyPadding(drawerBasePadding.withInsets(insets.left, insets.top, insets.right, insets.bottom))
            windowInsets
        }
    }

    private fun View.rootPadding() = RootPadding(paddingLeft, paddingTop, paddingRight, paddingBottom)

    private fun View.applyPadding(padding: RootPadding) {
        setPadding(padding.left, padding.top, padding.right, padding.bottom)
    }

    private fun toolbarIconButton(icon: Int, id: Int, description: String, action: () -> Unit): ImageButton {
        return ImageButton(this).apply {
            this.id = id
            contentDescription = description
            setImageResource(icon)
            setColorFilter(COLOR_ACCENT)
            scaleType = ImageView.ScaleType.CENTER
            setPadding(dp(10), dp(10), dp(10), dp(10))
            background = selectableBorderlessBackground()
            setOnClickListener { action() }
            layoutParams = LinearLayout.LayoutParams(
                dp(48),
                dp(48),
            ).apply {
                leftMargin = dp(4)
            }
        }
    }

    private fun buildNavigationDrawer() {
        drawerScrim = View(this).apply {
            id = R.id.drawer_scrim
            setBackgroundColor(COLOR_SCRIM)
            alpha = 0f
            visibility = View.GONE
            setOnClickListener { hideNavigationDrawer() }
            layoutParams = FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT,
            )
        }
        drawerPanel = LinearLayout(this).apply {
            id = R.id.drawer_panel
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(COLOR_DRAWER)
            setPadding(dp(18), dp(28), dp(18), dp(18))
            visibility = View.GONE
            elevation = dp(8).toFloat()
            translationX = -drawerWidth().toFloat()
            layoutParams = FrameLayout.LayoutParams(
                drawerWidth(),
                ViewGroup.LayoutParams.MATCH_PARENT,
                Gravity.START,
            )
            addView(TextView(this@MainActivity).apply {
                text = "Koko Tools"
                textSize = 20f
                setTypeface(Typeface.DEFAULT, Typeface.BOLD)
                setTextColor(COLOR_TEXT_PRIMARY)
                setPadding(dp(4), 0, dp(4), dp(16))
            })
            addView(drawerRow("Todo", R.id.tab_todo) {
                hideNavigationDrawer()
                showTodo()
            })
            addView(drawerRow("Notes", R.id.tab_notes) {
                hideNavigationDrawer()
                showNotes()
            })
            addView(drawerRow("Pages", R.id.tab_pages) {
                hideNavigationDrawer()
                showPages()
            })
            addView(drawerRow("Sync", R.id.tab_sync) {
                hideNavigationDrawer()
                showSync()
            })
            addView(drawerRow("Settings", R.id.tab_settings) {
                hideNavigationDrawer()
                showSettings()
            })
        }
        root.addView(drawerScrim)
        root.addView(drawerPanel)
    }

    private fun drawerRow(label: String, id: Int, action: () -> Unit): TextView {
        return TextView(this).apply {
            this.id = id
            text = label
            textSize = 17f
            setTextColor(COLOR_TEXT_PRIMARY)
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(12), 0, dp(12), 0)
            background = selectableFillBackground(COLOR_SURFACE_ALT)
            setOnClickListener { action() }
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(48),
            ).apply {
                bottomMargin = dp(8)
            }
        }
    }

    private fun showNavigationDrawer() {
        drawerScrim.visibility = View.VISIBLE
        drawerPanel.visibility = View.VISIBLE
        drawerScrim.animate().alpha(1f).setDuration(DRAWER_ANIMATION_MS).start()
        drawerPanel.animate().translationX(0f).setDuration(DRAWER_ANIMATION_MS).start()
    }

    private fun hideNavigationDrawer() {
        drawerScrim.animate().alpha(0f).setDuration(DRAWER_ANIMATION_MS).withEndAction {
            drawerScrim.visibility = View.GONE
        }.start()
        drawerPanel.animate().translationX(-drawerWidth().toFloat()).setDuration(DRAWER_ANIMATION_MS).withEndAction {
            drawerPanel.visibility = View.GONE
        }.start()
    }

    private fun isNavigationDrawerOpen(): Boolean {
        return ::drawerPanel.isInitialized && drawerPanel.visibility == View.VISIBLE
    }

    @Suppress("DEPRECATION", "OVERRIDE_DEPRECATION")
    override fun onBackPressed() {
        if (isNavigationDrawerOpen()) {
            hideNavigationDrawer()
            return
        }
        super.onBackPressed()
    }

    private fun drawerWidth(): Int {
        return dp(280)
    }

    private fun showCurrentActionsMenu() {
        when (currentScreen) {
            Screen.Notes -> showNotesActionsMenu(actionsButton)
            Screen.Pages -> showPagesActionsMenu(actionsButton)
            Screen.Todo -> showTodoActionsMenu(actionsButton)
            Screen.Sync -> showSyncActionsMenu(actionsButton)
            Screen.Settings -> showSettingsActionsMenu(actionsButton)
        }
    }

    private fun showLastScreen() {
        when (AndroidScreenState.normalize(settings.androidApp.lastScreen)) {
            AndroidScreenState.NOTES -> showNotes()
            AndroidScreenState.PAGES -> showPages()
            AndroidScreenState.SYNC -> showSync()
            AndroidScreenState.SETTINGS -> showSettings()
            else -> showTodo()
        }
    }

    private fun showScreen(screen: Screen) {
        val changed = currentScreen != screen
        currentScreen = screen
        val lastScreen = screen.settingValue
        if (settings.androidApp.lastScreen != lastScreen) {
            persistLocalSettings(settings.copy(androidApp = settings.androidApp.copy(lastScreen = lastScreen)))
        }
        if (changed) {
            syncCurrentScreenToFirebase()
        }
    }

    private fun showNotes() {
        showScreen(Screen.Notes)
        setScreenHeader("Notes", currentNoteLabel())
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(0, 0, 0, 0)

        noteListView = null
        noteSelector = null
        rawNoteSpellChecker?.close()
        rawNoteSpellChecker = null
        rawNoteEditor = null

        if (settings.notesApp.previewHidden) {
            noteEditor = null
            rawNoteEditor = buildRawNoteEditor()
            content.addView(rawNoteEditor)
        } else {
            noteEditor = HybridMarkdownEditor(
                this,
                palette,
                settings.notesApp.spellCheckEnabled,
                settings.notesApp.spellDictionaries,
            ).apply {
                background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, 0f)
                layoutParams = LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    0,
                    1f,
                )
                onMarkdownChanged = { scheduleNoteAutosave() }
            }
            content.addView(noteEditor)
        }

        refreshNotes()
    }

    private fun buildRawNoteEditor(): EditText {
        return EditText(this).apply {
            id = R.id.note_editor
            minLines = 10
            textSize = 16f
            gravity = Gravity.TOP or Gravity.START
            inputType = NoteEditorInputTypes.forSpellCheck(settings.notesApp.spellCheckEnabled)
            rawNoteSpellChecker = AndroidSpellChecker(this@MainActivity, this).also {
                it.setConfig(settings.notesApp.spellCheckEnabled, settings.notesApp.spellDictionaries)
            }
            setTextColor(COLOR_TEXT_PRIMARY)
            setHintTextColor(COLOR_TEXT_MUTED)
            setPadding(dp(14), dp(12), dp(14), dp(16))
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, 0f)
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                1f,
            )
            addTextChangedListener(MarkdownHighlightingTextWatcher(MarkdownHighlighter(palette.markdown)))
            addTextChangedListener(object : TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) = Unit
                override fun afterTextChanged(s: Editable?) {
                    scheduleNoteAutosave()
                    rawNoteSpellChecker?.onTextChanged()
                }
            })
            setOnTouchListener { view, event ->
                when (event.actionMasked) {
                    MotionEvent.ACTION_DOWN -> {
                        rawNoteTouchDownAt = event.eventTime
                        parent?.requestDisallowInterceptTouchEvent(true)
                    }
                    MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> parent?.requestDisallowInterceptTouchEvent(false)
                }
                val selecting = selectionStart != selectionEnd
                val longGesture = event.eventTime - rawNoteTouchDownAt >= ViewConfiguration.getLongPressTimeout()
                if (!selecting && !longGesture && rawNoteSpellChecker?.handleTouchEvent(event) == true) {
                    return@setOnTouchListener true
                }
                false
            }
        }
    }

    private fun showNotesActionsMenu(anchor: View) {
        PopupMenu(this, anchor).apply {
            menu.add("Open note").setOnMenuItemClickListener {
                showNotePicker()
                true
            }
            menu.add("New note").setOnMenuItemClickListener {
                promptNewNote()
                true
            }
            menu.add(if (settings.notesApp.previewHidden) "Turn rendering on" else "Turn rendering off")
                .setOnMenuItemClickListener {
                    toggleNoteRendering()
                    true
                }
            menu.add("Delete note").setOnMenuItemClickListener {
                confirmDeleteCurrentNote()
                true
            }
            menu.add("Open sync").setOnMenuItemClickListener {
                showSync()
                true
            }
            show()
        }
    }

    private fun refreshNotes() {
        noteList = notesRepository.listNotes()

        val selectedPath = currentNotePath.takeIf { path ->
            noteList.any { it.relativePath == path }
        } ?: noteList.firstOrNull()?.relativePath.orEmpty()

        if (selectedPath.isNotBlank()) {
            selectNote(selectedPath)
        } else {
            currentNotePath = ""
            setEditorTextFromNote("")
            loadedNoteText = ""
            updateNoteSelector()
        }
    }

    private fun selectNote(relativePath: String) {
        cancelPendingNoteAutosave()
        currentNotePath = relativePath
        loadedNoteText = notesRepository.read(relativePath)
        setEditorTextFromNote(loadedNoteText)
        persistSettings(
            settings.copy(notesApp = settings.notesApp.copy(currentNotePath = relativePath)),
        )
        updateNoteSelector()
    }

    private fun updateNoteSelector() {
        noteSelector?.text = if (currentNotePath.isBlank()) "No note selected" else currentNotePath
        if (currentScreen == Screen.Notes) {
            subtitleText.text = currentNoteLabel()
        }
    }

    private fun currentNoteLabel(): String {
        return currentNotePath.ifBlank { "No note selected" }
    }

    private fun setEditorTextFromNote(text: String) {
        suppressNoteAutosave = true
        try {
            noteEditor?.setMarkdown(text, 0)
            noteEditor?.scrollEditorToTop()
            rawNoteEditor?.setText(text)
            rawNoteEditor?.setSelection(0)
            rawNoteEditor?.post { rawNoteEditor?.scrollTo(0, 0) }
        } finally {
            suppressNoteAutosave = false
        }
    }

    private fun updateEditorTextFromRemoteNote(text: String) {
        suppressNoteAutosave = true
        try {
            noteEditor?.updateMarkdownPreservingState(text)
            rawNoteEditor?.let { editor ->
                val selectionStart = editor.selectionStart.coerceAtLeast(0)
                val selectionEnd = editor.selectionEnd.coerceAtLeast(0)
                val scrollX = editor.scrollX
                val scrollY = editor.scrollY
                val wasFocused = editor.hasFocus()
                editor.setText(text)
                val nextSelectionStart = selectionStart.coerceIn(0, editor.length())
                val nextSelectionEnd = selectionEnd.coerceIn(0, editor.length())
                editor.setSelection(
                    minOf(nextSelectionStart, nextSelectionEnd),
                    maxOf(nextSelectionStart, nextSelectionEnd),
                )
                if (wasFocused) {
                    editor.requestFocus()
                }
                editor.post { editor.scrollTo(scrollX, scrollY) }
            }
        } finally {
            suppressNoteAutosave = false
        }
    }

    private fun applyRemoteTextToCurrentNote(text: String): Boolean {
        val visibleText = noteEditorText()
        if (text == loadedNoteText && text == visibleText) return false
        if (text == visibleText) {
            loadedNoteText = text
            return false
        }
        loadedNoteText = text
        updateEditorTextFromRemoteNote(text)
        return true
    }

    private fun clearCurrentNoteFromRemoteDelete() {
        if (currentNotePath.isBlank() && loadedNoteText.isEmpty() && noteEditorText().isEmpty()) return
        currentNotePath = ""
        loadedNoteText = ""
        setEditorTextFromNote("")
        persistSettings(settings.copy(notesApp = settings.notesApp.copy(currentNotePath = "")))
        updateNoteSelector()
    }

    private fun toggleNoteRendering() {
        changeNoteRenderingEnabled(settings.notesApp.previewHidden)
    }

    private fun changeNoteRenderingEnabled(enabled: Boolean) {
        if (enabled == !settings.notesApp.previewHidden) return
        saveCurrentNoteSilently()
        persistSettings(
            settings.copy(
                notesApp = settings.notesApp.copy(previewHidden = !enabled),
            ),
        )
        if (currentScreen == Screen.Notes) {
            showNotes()
        } else if (currentScreen == Screen.Settings) {
            showSettings()
        }
    }

    private fun showNotePicker() {
        noteList = notesRepository.listNotes()
        var treeMode = false
        var rows: List<NotePickerRow> = emptyList()
        lateinit var dialog: AlertDialog

        val pickerLayout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(0, dp(8), 0, 0)
        }
        val toggleRow = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.END
            setPadding(0, 0, 0, dp(8))
        }
        val flatButton = dialogIconButton(
            icon = R.drawable.ic_view_list_24,
            id = R.id.note_picker_flat,
            description = "Flat note list",
        )
        val treeButton = dialogIconButton(
            icon = R.drawable.ic_account_tree_24,
            id = R.id.note_picker_tree,
            description = "Note tree",
        )
        toggleRow.addView(flatButton)
        toggleRow.addView(treeButton)

        val emptyText = TextView(this).apply {
            id = R.id.note_picker_empty
            text = "No notes"
            textSize = 15f
            setTextColor(COLOR_TEXT_SECONDARY)
            gravity = Gravity.CENTER
            visibility = View.GONE
            setPadding(dp(14), dp(24), dp(14), dp(24))
        }
        val pickerList = ListView(this).apply {
            id = R.id.note_picker_list
            divider = null
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(320),
            )
        }

        pickerLayout.addView(toggleRow)
        pickerLayout.addView(emptyText)
        pickerLayout.addView(pickerList)

        fun refreshPickerRows() {
            rows = if (treeMode) {
                NotePickerRows.tree(noteList, notePickerExpandedFolders)
            } else {
                NotePickerRows.flat(noteList)
            }
            val labels = rows.map { row -> "${"  ".repeat(row.depth)}${row.label}" }
            pickerList.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_1, labels)
            pickerList.visibility = if (rows.isEmpty()) View.GONE else View.VISIBLE
            emptyText.visibility = if (rows.isEmpty()) View.VISIBLE else View.GONE
            flatButton.isSelected = !treeMode
            treeButton.isSelected = treeMode
            flatButton.alpha = if (treeMode) 0.72f else 1f
            treeButton.alpha = if (treeMode) 1f else 0.72f
        }

        flatButton.setOnClickListener {
            treeMode = false
            refreshPickerRows()
        }
        treeButton.setOnClickListener {
            treeMode = true
            refreshPickerRows()
        }
        pickerList.setOnItemClickListener { _, _, position, _ ->
            val row = rows[position]
            if (row.folder) {
                if (notePickerExpandedFolders.contains(row.relativePath)) {
                    notePickerExpandedFolders.remove(row.relativePath)
                } else {
                    notePickerExpandedFolders.add(row.relativePath)
                }
                refreshPickerRows()
            } else {
                requestNoteSwitch(row.relativePath) {
                    dialog.dismiss()
                }
            }
        }

        dialog = AlertDialog.Builder(this)
            .setTitle("Open note")
            .setView(pickerLayout)
            .setNegativeButton("Cancel", null)
            .create()
        dialog.setOnShowListener { refreshPickerRows() }
        dialog.show()
    }

    private fun requestNoteSwitch(relativePath: String, afterSwitch: () -> Unit = {}) {
        if (relativePath == currentNotePath) {
            afterSwitch()
            return
        }
        val previousPath = currentNotePath
        saveCurrentNoteSilently()
        if (previousPath.isNotBlank()) {
            applyPendingRemoteNoteToFile(previousPath, notesRepository.read(previousPath), updateEditor = false)
        }
        applyPendingRemoteNoteToFile(relativePath, notesRepository.read(relativePath), updateEditor = false)
        selectNote(relativePath)
        afterSwitch()
    }

    private fun hasUnsavedNoteChanges(): Boolean {
        return noteEditorText() != loadedNoteText
    }

    private fun promptNewNote() {
        var selectedFolder = ""
        val folders = notesRepository.listFolders()
        lateinit var folderButton: Button
        folderButton = commandButton("Folder: Root", View.generateViewId()) {
            showNewNoteFolderPicker(folders, selectedFolder) { folder ->
                selectedFolder = folder
                folderButton.text = "Folder: ${folder.ifBlank { "Root" }}"
            }
        }
        val input = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_TEXT
            hint = "folder/name.md"
            setSingleLine(true)
        }
        val form = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(8), dp(4), dp(8), dp(4))
            addView(folderButton)
            addView(input)
        }
        AlertDialog.Builder(this)
            .setTitle("New note")
            .setView(form)
            .setPositiveButton("Create") { _, _ ->
                val path = NotesRepository.buildNewNotePath(selectedFolder, input.text.toString())
                currentNotePath = path
                notesRepository.save(path, "")
                pushNoteToFirebase(path, "")
                loadedNoteText = ""
                refreshNotes()
                selectNote(path)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun showNewNoteFolderPicker(folders: List<String>, selectedFolder: String, onSelect: (String) -> Unit) {
        val labels = listOf("Root") + folders
        val selectedIndex = folders.indexOf(selectedFolder).let { if (it >= 0) it + 1 else 0 }
        AlertDialog.Builder(this)
            .setTitle("Folder")
            .setSingleChoiceItems(labels.toTypedArray(), selectedIndex) { dialog, which ->
                onSelect(if (which == 0) "" else folders[which - 1])
                dialog.dismiss()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun saveCurrentNote() {
        saveCurrentNoteInternal(showToast = true, refreshList = true)
    }

    private fun saveCurrentNoteSilently() {
        cancelPendingNoteAutosave()
        val text = noteEditorText()
        if (currentNotePath.isBlank() && text.isEmpty()) return
        if (currentNotePath.isNotBlank() && text == loadedNoteText) return
        saveCurrentNoteInternal(showToast = false, refreshList = false)
    }

    private fun saveCurrentNoteInternal(showToast: Boolean, refreshList: Boolean) {
        val path = currentNotePath.ifBlank { "untitled.md" }
        val text = noteEditorText()
        val saved = notesRepository.save(path, text)
        currentNotePath = saved.relativePath
        persistSettings(
            settings.copy(notesApp = settings.notesApp.copy(currentNotePath = saved.relativePath)),
        )
        loadedNoteText = text
        pushNoteToFirebase(saved.relativePath, text)
        if (refreshList) {
            refreshNotes()
        } else {
            updateNoteSelector()
        }
        if (showToast) {
            Toast.makeText(this, "Saved ${saved.relativePath}", Toast.LENGTH_SHORT).show()
        }
    }

    private fun scheduleNoteAutosave() {
        if (suppressNoteAutosave || currentScreen != Screen.Notes) return
        pendingNoteAutosave?.let { noteAutosaveHandler.removeCallbacks(it) }
        val autosave = Runnable {
            pendingNoteAutosave = null
            if (hasUnsavedNoteChanges()) {
                saveCurrentNoteInternal(showToast = false, refreshList = false)
            }
        }
        pendingNoteAutosave = autosave
        noteAutosaveHandler.postDelayed(autosave, NOTE_AUTOSAVE_DELAY_MS)
    }

    private fun cancelPendingNoteAutosave() {
        pendingNoteAutosave?.let { noteAutosaveHandler.removeCallbacks(it) }
        pendingNoteAutosave = null
    }

    private fun noteEditorText(): String {
        return noteEditor?.getMarkdown() ?: rawNoteEditor?.text?.toString().orEmpty()
    }

    private fun confirmDeleteCurrentNote() {
        val path = currentNotePath
        if (path.isBlank()) return
        AlertDialog.Builder(this)
            .setTitle("Delete note")
            .setMessage("Delete $path?")
            .setPositiveButton("Delete") { _, _ ->
                runCatching {
                    completeLocalNoteDeletion(
                        deleteLocal = { notesRepository.delete(path) },
                        afterDelete = {
                            pushNoteDeleteToFirebase(path)
                            currentNotePath = ""
                            loadedNoteText = ""
                            persistSettings(settings.copy(notesApp = settings.notesApp.copy(currentNotePath = "")))
                            refreshNotes()
                        },
                    )
                }.onSuccess { deleted ->
                    if (!deleted) {
                        Toast.makeText(this, "Note was not deleted locally", Toast.LENGTH_SHORT).show()
                    }
                }.onFailure { error ->
                    Toast.makeText(this, "Local note delete failed: ${error.message}", Toast.LENGTH_LONG).show()
                }
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun formatBytes(size: Int): String {
        if (size < 1024) return "$size B"
        if (size < 1024 * 1024) return "${size / 1024} KB"
        return "${size / (1024 * 1024)} MB"
    }

    private fun showPages() {
        showScreen(Screen.Pages)
        setScreenHeader("Pages", "Reading progress converter")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(dp(16), dp(16), dp(16), dp(16))
        val pages = settings.pagesApp

        firstInput = numberInput(R.id.pages_first, pages.firstBook)
        readInput = numberInput(R.id.pages_read, pages.readPages)
        secondInput = numberInput(R.id.pages_second, pages.secondBook)
        resultText = resultPanel()

        content.addView(sectionTitle("Book pages"))
        content.addView(formRow("First book", firstInput))
        content.addView(formRow("Read pages", readInput))
        content.addView(formRow("Other book", secondInput))
        content.addView(resultText)

        val watcher = object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                recalculatePages(userEdit = true)
            }
            override fun afterTextChanged(s: Editable?) = Unit
        }
        firstInput?.addTextChangedListener(watcher)
        readInput?.addTextChangedListener(watcher)
        secondInput?.addTextChangedListener(watcher)
        recalculatePages()
    }

    private fun showPagesActionsMenu(anchor: View) {
        PopupMenu(this, anchor).apply {
            menu.add("Recalculate").setOnMenuItemClickListener {
                recalculatePages(userEdit = true)
                true
            }
            menu.add("Open notes").setOnMenuItemClickListener {
                showNotes()
                true
            }
            show()
        }
    }

    private fun numberInput(id: Int, value: Int): EditText {
        return EditText(this).apply {
            this.id = id
            inputType = InputType.TYPE_CLASS_NUMBER
            setText(value.toString())
            selectAll()
            textSize = 18f
            setTextColor(COLOR_TEXT_PRIMARY)
            setHintTextColor(COLOR_TEXT_MUTED)
            setPadding(dp(14), 0, dp(14), 0)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(10).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(52),
            )
        }
    }

    private fun recalculatePages(userEdit: Boolean = false) {
        val result = pagesCalculator.calculate(
            readInput?.text?.toString().orEmpty(),
            firstInput?.text?.toString().orEmpty(),
            secondInput?.text?.toString().orEmpty(),
        )
        resultText?.text = result.label
        if (userEdit) {
            pagesLocalEditAtMs = System.currentTimeMillis()
            pagesLocalEditUntilMs = pagesLocalEditAtMs + 2_500L
        }
        val nextPages = PagesSettings(
            firstBook = result.firstBookPages,
            secondBook = result.secondBookPages,
            readPages = result.readPages,
        )
        if (nextPages == settings.pagesApp) return
        persistSettings(
            settings.copy(
                pagesApp = nextPages,
            ),
        )
    }

    private fun showTodo() {
        showScreen(Screen.Todo)
        setScreenHeader("Todo", if (todoMoveMode) "Move tasks" else "Tasks and archive")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(dp(16), dp(16), dp(16), dp(16))
        todoStore = todoRepository.load()
        todoMoveRows.clear()
        todoDraggingId = null

        val inputRow = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            ).apply { bottomMargin = dp(10) }
        }
        val input = EditText(this).apply {
            id = R.id.todo_input
            hint = "New todo"
            setSingleLine(true)
            inputType = InputType.TYPE_CLASS_TEXT
            setText(todoDraftText)
            setSelection(text.length)
            setTextColor(COLOR_TEXT_PRIMARY)
            setHintTextColor(COLOR_TEXT_MUTED)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(10).toFloat())
            setPadding(dp(12), 0, dp(12), 0)
            layoutParams = LinearLayout.LayoutParams(0, dp(48), 1f)
            addTextChangedListener(object : TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                    todoDraftText = s?.toString().orEmpty()
                }
                override fun afterTextChanged(s: Editable?) = Unit
            })
        }
        val addButton = commandButton("Add", R.id.todo_add) {
            todoStore = todoRepository.add(input.text.toString())
            pushTodosToFirebase()
            todoDraftText = ""
            input.setText("")
            showTodo()
        }.apply {
            layoutParams = LinearLayout.LayoutParams(dp(84), dp(48)).apply {
                leftMargin = dp(8)
            }
        }
        inputRow.addView(input)
        inputRow.addView(addButton)
        content.addView(inputRow)

        val scroll = ScrollView(this).apply {
            id = R.id.todo_list
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                1f,
            )
        }
        val list = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
        }
        scroll.addView(list)
        content.addView(scroll)

        TodoScreenRows.activeSections(todoStore).forEach { section ->
            addTodoSection(list, section, archived = false)
        }
        list.addView(sectionTitle("Archive"))
        val groups = TodoRepository.archiveGroups(todoStore)
        val archiveMonths = TodoRepository.archiveMonths(todoStore)
        if (archiveMonths.isEmpty()) {
            list.addView(emptySectionText("No archived todos"))
        } else {
            archiveMonths.forEach { month ->
                val items = groups[month].orEmpty()
                list.addView(todoArchiveMonthRow(month, items.size))
                if (expandedTodoArchiveMonths.contains(month)) {
                    items.forEach { list.addView(todoRow(it, archived = true)) }
                }
            }
        }
        scheduleTodoBoundaryRefresh()
    }

    private fun todoArchiveMonthRow(month: String, cachedCount: Int): TextView {
        return TextView(this).apply {
            val expanded = expandedTodoArchiveMonths.contains(month)
            val loading = loadingTodoArchiveMonths.contains(month)
            text = when {
                loading -> "$month - loading"
                expanded -> "$month - $cachedCount archived"
                else -> month
            }
            textSize = 14f
            setTypeface(Typeface.DEFAULT, Typeface.BOLD)
            setTextColor(COLOR_TEXT_PRIMARY)
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(12), 0, dp(12), 0)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(8).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(44),
            ).apply { bottomMargin = dp(6) }
            setOnClickListener { openTodoArchiveMonth(month) }
        }
    }

    private fun openTodoArchiveMonth(month: String) {
        if (expandedTodoArchiveMonths.contains(month)) {
            expandedTodoArchiveMonths.remove(month)
            showTodoPreservingScroll()
            return
        }
        if (!firebaseSyncRepository.configured(settings.firebase)) {
            expandedTodoArchiveMonths.add(month)
            showTodoPreservingScroll()
            return
        }
        if (!loadingTodoArchiveMonths.add(month)) return
        showTodoPreservingScroll()
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread {
                    loadingTodoArchiveMonths.remove(month)
                    setSyncError("Firebase login required")
                    showTodoPreservingScroll()
                }
                return@Thread
            }
            runCatching {
                val remoteItems = firebaseSyncRepository.pullTodoArchiveMonth(settings.firebase, month, session)
                val local = todoRepository.load()
                TodoRepository.mergeArchiveMonth(local, month, remoteItems)
            }.onSuccess { merged ->
                todoRepository.save(merged)
                runOnUiThread {
                    todoStore = merged
                    loadingTodoArchiveMonths.remove(month)
                    expandedTodoArchiveMonths.add(month)
                    setSyncSuccess("Firebase todo archive loaded: $month")
                    showTodoPreservingScroll()
                }
            }.onFailure { error ->
                runOnUiThread {
                    loadingTodoArchiveMonths.remove(month)
                    setSyncError("Firebase todo archive load failed: ${error.message}")
                    showTodoPreservingScroll()
                }
            }
        }.start()
    }

    private fun showTodoPreservingScroll() {
        val scrollY = findViewById<ScrollView?>(R.id.todo_list)?.scrollY ?: 0
        showTodo()
        findViewById<ScrollView?>(R.id.todo_list)?.post {
            findViewById<ScrollView?>(R.id.todo_list)?.scrollTo(0, scrollY)
        }
    }

    private fun addTodoSection(parent: LinearLayout, section: TodoDisplaySection, archived: Boolean) {
        parent.addView(sectionTitle(section.title))
        if (section.items.isEmpty()) {
            parent.addView(emptySectionText(section.emptyText))
            return
        }
        section.items.forEach { parent.addView(todoRow(it, archived)) }
    }

    private fun emptySectionText(text: String): TextView {
        return TextView(this).apply {
            this.text = text
            textSize = 14f
            setTextColor(COLOR_TEXT_MUTED)
            setPadding(dp(12), dp(8), dp(12), dp(8))
        }
    }

    private fun todoRow(item: TodoItem, archived: Boolean): LinearLayout {
        return LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(8), dp(6), dp(4), dp(6))
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(8).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            ).apply { bottomMargin = dp(6) }

            val checked = item.checkedAt != null || item.status == TodoRepository.STATUS_DONE || item.status == TodoRepository.STATUS_ARCHIVED
            val reorderable = canReorderTodo(item, archived)
            val checkBox = CheckBox(this@MainActivity).apply {
                isChecked = checked
                isEnabled = !archived
                buttonTintList = ColorStateList.valueOf(COLOR_ACCENT)
                layoutParams = LinearLayout.LayoutParams(dp(48), dp(48))
                setOnClickListener {
                    todoStore = todoRepository.toggle(item.id)
                    pushTodosToFirebase()
                    showTodo()
                }
            }
            val label = TextView(this@MainActivity).apply {
                text = item.text
                textSize = 16f
                setTextColor(if (archived) COLOR_TEXT_MUTED else COLOR_TEXT_PRIMARY)
                if (checked) paintFlags = paintFlags or Paint.STRIKE_THRU_TEXT_FLAG
                minLines = 1
                layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
            }
            addView(checkBox)
            addView(label)
            if (reorderable && todoMoveMode) {
                todoMoveRows[item.id] = this
                addView(todoDragHandle(item))
            }
            if (item.status == TodoRepository.STATUS_TODO && item.checkedAt == null && !archived && !todoMoveMode) {
                val target = if (TodoRepository.normalizeTerm(item.term) == TodoRepository.TERM_LONG) "short term" else "long term"
                addView(todoIconButton(R.drawable.ic_swap_vert_24, "Move to $target") { moveTodoTerm(item) })
                addView(todoIconButton(R.drawable.ic_edit_24, "Edit todo") { promptEditTodo(item) })
            }
        }
    }

    private fun moveTodoTerm(item: TodoItem) {
        todoStore = todoRepository.moveTerm(item.id)
        pushTodosToFirebase()
        showTodo()
    }

    private fun canReorderTodo(item: TodoItem, archived: Boolean): Boolean {
        return !archived && item.status == TodoRepository.STATUS_TODO && item.checkedAt == null
    }

    private fun todoIconButton(icon: Int, description: String, action: () -> Unit): ImageButton {
        return ImageButton(this).apply {
            contentDescription = description
            setImageResource(icon)
            setColorFilter(COLOR_ACCENT)
            scaleType = ImageView.ScaleType.CENTER
            setPadding(dp(9), dp(9), dp(9), dp(9))
            background = selectableBorderlessBackground()
            setOnClickListener { action() }
            layoutParams = LinearLayout.LayoutParams(dp(44), dp(44)).apply {
                leftMargin = dp(6)
            }
        }
    }

    private fun todoDragHandle(item: TodoItem): ImageButton {
        return ImageButton(this).apply {
            contentDescription = "Move todo"
            setImageResource(R.drawable.ic_drag_handle_24)
            setColorFilter(COLOR_ACCENT)
            scaleType = ImageView.ScaleType.CENTER
            setPadding(dp(9), dp(9), dp(9), dp(9))
            background = selectableBorderlessBackground()
            setOnTouchListener(todoMoveTouchListener(item))
            layoutParams = LinearLayout.LayoutParams(dp(44), dp(44)).apply {
                leftMargin = dp(6)
            }
        }
    }

    private fun todoMoveTouchListener(item: TodoItem): View.OnTouchListener {
        return View.OnTouchListener { view, event ->
            when (event.actionMasked) {
                MotionEvent.ACTION_DOWN -> {
                    todoDraggingId = item.id
                    view.alpha = 0.58f
                    view.parent?.requestDisallowInterceptTouchEvent(true)
                    true
                }
                MotionEvent.ACTION_MOVE -> {
                    highlightTodoMoveTarget(event.rawX.toInt(), event.rawY.toInt())
                    true
                }
                MotionEvent.ACTION_UP -> {
                    val draggedId = todoDraggingId
                    val targetId = todoMoveTargetAt(event.rawX.toInt(), event.rawY.toInt())
                    clearTodoMoveHighlight()
                    todoDraggingId = null
                    view.alpha = 1f
                    view.parent?.requestDisallowInterceptTouchEvent(false)
                    if (draggedId != null && targetId != null && draggedId != targetId) {
                        todoStore = todoRepository.moveTo(draggedId, targetId)
                        pushTodosToFirebase()
                        showTodo()
                    }
                    true
                }
                MotionEvent.ACTION_CANCEL -> {
                    clearTodoMoveHighlight()
                    todoDraggingId = null
                    view.alpha = 1f
                    view.parent?.requestDisallowInterceptTouchEvent(false)
                    true
                }
                else -> true
            }
        }
    }

    private fun todoMoveTargetAt(rawX: Int, rawY: Int): String? {
        val rect = Rect()
        return todoMoveRows.entries.firstOrNull { (_, row) ->
            row.getGlobalVisibleRect(rect) && rect.contains(rawX, rawY)
        }?.key
    }

    private fun highlightTodoMoveTarget(rawX: Int, rawY: Int) {
        val draggedId = todoDraggingId
        val targetId = todoMoveTargetAt(rawX, rawY)
        todoMoveRows.forEach { (id, row) ->
            if (id == targetId && id != draggedId) {
                row.background = todoMoveTargetBackground()
            } else {
                row.background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(8).toFloat())
            }
        }
    }

    private fun clearTodoMoveHighlight() {
        todoMoveRows.values.forEach {
            it.background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(8).toFloat())
        }
    }

    private fun todoMoveTargetBackground(): GradientDrawable {
        return if (palette.lightSystemBars) {
            roundedStroke(COLOR_SURFACE_ALT, COLOR_ACCENT, dp(8).toFloat())
        } else {
            roundedStroke(COLOR_TEXT_PRIMARY, COLOR_ACCENT, dp(8).toFloat())
        }
    }

    private fun promptEditTodo(item: TodoItem) {
        val input = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_TEXT
            setText(item.text)
            setSingleLine(true)
            selectAll()
        }
        AlertDialog.Builder(this)
            .setTitle("Edit todo")
            .setView(input)
            .setPositiveButton("Save") { _, _ ->
                todoStore = todoRepository.edit(item.id, input.text.toString())
                pushTodosToFirebase()
                showTodo()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun showTodoActionsMenu(anchor: View) {
        PopupMenu(this, anchor).apply {
            menu.add(if (todoMoveMode) "Done moving" else "Move").setOnMenuItemClickListener {
                todoMoveMode = !todoMoveMode
                showTodo()
                true
            }
            menu.add("Open sync").setOnMenuItemClickListener {
                showSync()
                true
            }
            show()
        }
    }

    private fun scheduleTodoBoundaryRefresh() {
        todoRefreshRunnable?.let { noteAutosaveHandler.removeCallbacks(it) }
        val hasPending = TodoRepository.activeItems(todoStore).any { it.checkedAt != null }
        if (!hasPending) return
        val refresh = Runnable {
            todoRefreshRunnable = null
            if (currentScreen == Screen.Todo) showTodo()
        }
        todoRefreshRunnable = refresh
        noteAutosaveHandler.postDelayed(refresh, (TodoRepository.CHECKED_DELAY_SECONDS + 1L) * 1000L)
    }

    private fun showSync() {
        showScreen(Screen.Sync)
        setScreenHeader("Sync", "Firebase sync")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(dp(16), dp(16), dp(16), dp(16))

        syncStatusText = TextView(this).apply {
            id = R.id.sync_status
            text = currentSyncStatus()
            textSize = 15f
            setTextColor(COLOR_TEXT_PRIMARY)
            setPadding(dp(14), dp(12), dp(14), dp(12))
            background = roundedFill(COLOR_STATUS, dp(10).toFloat())
        }
        content.addView(syncStatusText)
        content.addView(sectionTitle("Firebase"))
        content.addView(commandButton(if (settings.firebase.enabled) "Disable Firebase realtime" else "Enable Firebase realtime", View.generateViewId()) {
            val next = settings.firebase.copy(enabled = !settings.firebase.enabled, realtime = true)
            persistSettings(settings.copy(firebase = next))
            if (next.enabled) startFirebaseRealtimeIfEnabled()
            showSync()
        })
        content.addView(commandButton("Create Firebase account", View.generateViewId()) {
            promptFirebaseLogin(register = true)
        })
        content.addView(commandButton("Login Firebase", View.generateViewId()) {
            promptFirebaseLogin(register = false)
        })
        content.addView(commandButton("Login Firebase with Google", View.generateViewId()) {
            loginFirebaseWithGoogle()
        })
        content.addView(commandButton("Sync to Firebase", View.generateViewId()) {
            syncToFirebase()
        })
        if (BuildConfig.DEBUG || !FirebaseDefaults.bundled.ready) {
            content.addView(commandButton("Advanced custom Firebase config", View.generateViewId()) {
                promptFirebaseConfig()
            })
        }
    }

    private fun showSyncActionsMenu(anchor: View) {
        PopupMenu(this, anchor).apply {
            menu.add("Sync to Firebase").setOnMenuItemClickListener {
                syncToFirebase()
                true
            }
            show()
        }
    }

    private fun showSettings() {
        showScreen(Screen.Settings)
        setScreenHeader("Settings", "App preferences")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(0, 0, 0, 0)

        val scroll = ScrollView(this).apply {
            isFillViewport = true
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT,
            )
        }
        val settingsBody = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), dp(16), dp(16), dp(16))
            layoutParams = FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            )
        }
        scroll.addView(settingsBody)
        content.addView(scroll)

        settingsBody.addView(sectionTitle("Theme"))
        val group = RadioGroup(this).apply {
            id = R.id.settings_theme_group
            orientation = RadioGroup.VERTICAL
            setPadding(0, dp(4), 0, dp(8))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            )
        }
        group.addView(themeRadioButton(ThemeMode.Light, R.id.settings_theme_light))
        group.addView(themeRadioButton(ThemeMode.Dark, R.id.settings_theme_dark))
        group.addView(themeRadioButton(ThemeMode.System, R.id.settings_theme_system))
        group.check(themeRadioId(settings.androidApp.themeMode))
        group.setOnCheckedChangeListener { _, checkedId ->
            val mode = themeModeForRadioId(checkedId)
            if (mode != settings.androidApp.themeMode) {
                changeThemeMode(mode)
            }
        }
        settingsBody.addView(group)

        settingsBody.addView(sectionTitle("Notes"))
        val richRendering = CheckBox(this).apply {
            id = R.id.settings_note_rendering
            text = "Rich text rendering"
            textSize = 16f
            setTextColor(COLOR_TEXT_PRIMARY)
            buttonTintList = ColorStateList.valueOf(COLOR_ACCENT)
            background = selectableFillBackground(COLOR_SURFACE)
            setPadding(dp(12), 0, dp(12), 0)
            isChecked = !settings.notesApp.previewHidden
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(48),
            ).apply {
                topMargin = dp(4)
                bottomMargin = dp(8)
            }
            setOnCheckedChangeListener { _, checked ->
                if (checked != !settings.notesApp.previewHidden) {
                    changeNoteRenderingEnabled(checked)
                }
            }
        }
        settingsBody.addView(richRendering)
        val spellCheck = CheckBox(this).apply {
            id = R.id.settings_spell_check
            text = "Spell checking"
            textSize = 16f
            setTextColor(COLOR_TEXT_PRIMARY)
            buttonTintList = ColorStateList.valueOf(COLOR_ACCENT)
            background = selectableFillBackground(COLOR_SURFACE)
            setPadding(dp(12), 0, dp(12), 0)
            isChecked = settings.notesApp.spellCheckEnabled
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(48),
            ).apply {
                topMargin = dp(4)
                bottomMargin = dp(8)
            }
            setOnCheckedChangeListener { _, checked ->
                if (checked != settings.notesApp.spellCheckEnabled) {
                    changeSpellCheckEnabled(checked)
                }
            }
        }
        settingsBody.addView(spellCheck)

        settingsBody.addView(sectionTitle("Spell dictionaries"))
        val selectedSpellDictionaries = SpellLanguages.normalizeCodes(settings.notesApp.spellDictionaries).toSet()
        SpellLanguages.supported.forEach { language ->
            settingsBody.addView(spellDictionaryCheckBox(language, selectedSpellDictionaries))
        }

        settingsBody.addView(sectionTitle("Privacy"))
        settingsBody.addView(commandButton("Privacy policy", R.id.settings_privacy_policy) {
            openExternalUrl(PRIVACY_POLICY_URL)
        })
        settingsBody.addView(commandButton("Delete account or data", R.id.settings_delete_account) {
            openExternalUrl(ACCOUNT_DELETION_URL)
        })
    }

    private fun showSettingsActionsMenu(anchor: View) {
        PopupMenu(this, anchor).apply {
            menu.add("Open notes").setOnMenuItemClickListener {
                showNotes()
                true
            }
            show()
        }
    }

    private fun themeRadioButton(mode: ThemeMode, id: Int): RadioButton {
        return RadioButton(this).apply {
            this.id = id
            text = mode.label
            textSize = 16f
            setTextColor(COLOR_TEXT_PRIMARY)
            buttonTintList = ColorStateList.valueOf(COLOR_ACCENT)
            background = selectableFillBackground(COLOR_SURFACE)
            setPadding(dp(12), 0, dp(12), 0)
            layoutParams = RadioGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(48),
            ).apply {
                bottomMargin = dp(8)
            }
        }
    }

    private fun spellDictionaryCheckBox(language: SpellLanguage, selectedCodes: Set<String>): CheckBox {
        return CheckBox(this).apply {
            text = language.label
            textSize = 16f
            setTextColor(if (settings.notesApp.spellCheckEnabled) COLOR_TEXT_PRIMARY else COLOR_TEXT_MUTED)
            buttonTintList = ColorStateList.valueOf(COLOR_ACCENT)
            background = selectableFillBackground(COLOR_SURFACE)
            setPadding(dp(12), 0, dp(12), 0)
            isEnabled = settings.notesApp.spellCheckEnabled
            isChecked = language.code in selectedCodes
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(44),
            ).apply {
                bottomMargin = dp(6)
            }
            setOnCheckedChangeListener { _, checked ->
                changeSpellDictionaryEnabled(language.code, checked)
            }
        }
    }

    private fun themeRadioId(mode: ThemeMode): Int {
        return when (mode) {
            ThemeMode.Light -> R.id.settings_theme_light
            ThemeMode.Dark -> R.id.settings_theme_dark
            ThemeMode.System -> R.id.settings_theme_system
        }
    }

    private fun themeModeForRadioId(id: Int): ThemeMode {
        return when (id) {
            R.id.settings_theme_light -> ThemeMode.Light
            R.id.settings_theme_dark -> ThemeMode.Dark
            else -> ThemeMode.System
        }
    }

    private fun changeThemeMode(mode: ThemeMode) {
        saveCurrentNoteSilently()
        persistSettings(settings.copy(androidApp = settings.androidApp.copy(themeMode = mode)))
        palette = resolvePalette()
        applySystemBars()
        buildRoot()
        showCurrentScreen()
    }

    private fun showCurrentScreen() {
        when (currentScreen) {
            Screen.Notes -> showNotes()
            Screen.Pages -> showPages()
            Screen.Todo -> showTodo()
            Screen.Sync -> showSync()
            Screen.Settings -> showSettings()
        }
    }

    private fun changeSpellCheckEnabled(enabled: Boolean) {
        persistSettings(settings.copy(notesApp = settings.notesApp.copy(spellCheckEnabled = enabled)))
        noteEditor?.setSpellCheckEnabled(enabled)
        rawNoteEditor?.inputType = NoteEditorInputTypes.forSpellCheck(enabled)
        rawNoteSpellChecker?.setConfig(enabled, settings.notesApp.spellDictionaries)
        if (currentScreen == Screen.Settings) showSettings()
    }

    private fun changeSpellDictionaryEnabled(code: String, enabled: Boolean) {
        val current = SpellLanguages.normalizeCodes(settings.notesApp.spellDictionaries).toMutableList()
        if (enabled) {
            if (code !in current) current.add(code)
        } else {
            current.remove(code)
        }
        val nextCodes = current.ifEmpty { mutableListOf("en") }
        persistSettings(settings.copy(notesApp = settings.notesApp.copy(spellDictionaries = nextCodes)))
        noteEditor?.setSpellDictionaries(nextCodes)
        rawNoteSpellChecker?.setConfig(settings.notesApp.spellCheckEnabled, nextCodes)
        if (currentScreen == Screen.Settings) showSettings()
    }

    private fun hideKeyboardForView(view: View) {
        val inputMethodManager = getSystemService(INPUT_METHOD_SERVICE) as? InputMethodManager
        inputMethodManager?.hideSoftInputFromWindow(view.windowToken, 0)
    }

    private fun openExternalUrl(url: String) {
        runCatching {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
        }.onFailure {
            Toast.makeText(this, "No browser available", Toast.LENGTH_SHORT).show()
        }
    }

    private fun persistSettings(next: AppSettings) {
        settings = next
        settingsRepository.save(settings)
        updateSyncProblemIndicator()
        scheduleSharedSettingsPush()
    }

    private fun persistLocalSettings(next: AppSettings) {
        settings = next
        settingsRepository.save(settings)
        updateSyncProblemIndicator()
    }

    private fun startFirebaseRealtimeIfEnabled() {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            firebaseSession = firebaseSyncRepository.currentSession(settings.firebase)
            runOnUiThread { scheduleFirebasePull() }
        }.start()
    }

    private fun freshFirebaseSession(): FirebaseSession? {
        val session = firebaseSyncRepository.currentSession(settings.firebase)
        if (session != null) {
            firebaseSession = session
            val workspaceSettings = firebaseSyncRepository.ensurePersonalWorkspace(settings.firebase, session)
            if (workspaceSettings != settings.firebase) {
                settings = settings.copy(firebase = workspaceSettings)
                settingsRepository.save(settings)
            }
        }
        return session
    }

    private fun scheduleFirebasePull() {
        firebasePullRunnable?.let { noteAutosaveHandler.removeCallbacks(it) }
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        val runnable = Runnable {
            firebasePullRunnable = null
            pullFirebaseRealtimeData()
            scheduleFirebasePull()
        }
        firebasePullRunnable = runnable
        noteAutosaveHandler.postDelayed(runnable, FIREBASE_PULL_INTERVAL_MS)
    }

    private fun syncCurrentScreenToFirebase(force: Boolean = false) {
        syncScreenToFirebase(currentScreen, force)
    }

    private fun syncScreenToFirebase(screen: Screen, force: Boolean = false) {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        val scope = screen.syncScope
        synchronized(firebaseViewSyncLock) {
            if (!force && firebaseViewSyncInFlight.contains(scope)) return
            if (!firebaseViewSyncInFlight.add(scope)) return
        }
        if (scope == FirebaseViewSyncScope.Full) {
            syncToFirebase()
            synchronized(firebaseViewSyncLock) {
                firebaseViewSyncInFlight.remove(scope)
            }
            return
        }
        if (scope == FirebaseViewSyncScope.Notes) {
            saveCurrentNoteSilently()
        }
        setSyncStatus("Firebase ${scope.label} sync started", transient = true)
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                finishViewSync(scope)
                return@Thread
            }
            when (scope) {
                FirebaseViewSyncScope.Todos -> syncTodoView(session)
                FirebaseViewSyncScope.Notes -> syncNotesView(session)
                FirebaseViewSyncScope.Settings -> syncSettingsView(session)
                FirebaseViewSyncScope.Full -> finishViewSync(scope)
            }
        }.start()
    }

    private fun finishViewSync(scope: FirebaseViewSyncScope) {
        synchronized(firebaseViewSyncLock) {
            firebaseViewSyncInFlight.remove(scope)
        }
    }

    private fun syncTodoView(session: FirebaseSession) {
        runCatching {
            val local = todoRepository.load()
            val merged = firebaseSyncRepository.pullTodos(settings.firebase, local, session)
            val changed = merged != local
            if (changed) todoRepository.save(merged)
            firebaseSyncRepository.pushTodos(settings.firebase, todoRepository.load(), session)
            changed to merged
        }.onSuccess { (changed, merged) ->
            runOnUiThread {
                todoStore = merged
                if (currentScreen == Screen.Todo && changed && canRebuildTodoAfterRemotePull()) {
                    showTodoPreservingScroll()
                }
                setSyncSuccess("Firebase todo view synced", transient = true)
            }
        }.onFailure { error ->
            runOnUiThread { setSyncError("Firebase todo view sync failed: ${error.message}", transient = true) }
        }
        finishViewSync(FirebaseViewSyncScope.Todos)
    }

    private fun syncNotesView(session: FirebaseSession) {
        runCatching {
            firebaseSyncRepository.pullNotes(settings.firebase, session)
        }.onSuccess { remoteNotes ->
            runOnUiThread {
                applyRemoteNotes(remoteNotes)
                if (currentScreen == Screen.Notes) refreshNotes()
                setSyncSuccess("Firebase notes view synced", transient = true)
                finishViewSync(FirebaseViewSyncScope.Notes)
            }
        }.onFailure { error ->
            runOnUiThread { setSyncError("Firebase notes view sync failed: ${error.message}", transient = true) }
            finishViewSync(FirebaseViewSyncScope.Notes)
        }
    }

    private fun syncSettingsView(session: FirebaseSession) {
        runCatching {
            firebaseSyncRepository.pullSharedSettings(settings.firebase, session)
        }.onSuccess { shared ->
            runOnUiThread {
                val localEditActive = hasActiveLocalEdit()
                if (shared != null && !localEditActive && shared.rev > pagesLocalEditAtMs) {
                    settings = SettingsRepository.applySharedSettings(settings, shared.values)
                    settingsRepository.save(settings)
                    firebaseSyncRepository.markSharedSettingsSynced(settings.firebase, shared.values)
                    showCurrentScreen()
                }
                Thread {
                    runCatching {
                        firebaseSyncRepository.pushSharedSettings(settings.firebase, settings, session)
                    }.onSuccess {
                        runOnUiThread { setSyncSuccess("Firebase settings view synced", transient = true) }
                    }.onFailure { error ->
                        runOnUiThread { setSyncError("Firebase settings view push failed: ${error.message}", transient = true) }
                    }
                    finishViewSync(FirebaseViewSyncScope.Settings)
                }.start()
            }
        }.onFailure { error ->
            runOnUiThread { setSyncError("Firebase settings view sync failed: ${error.message}", transient = true) }
            finishViewSync(FirebaseViewSyncScope.Settings)
        }
    }

    private fun promptFirebaseConfig() {
        val form = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(8), dp(4), dp(8), dp(4))
        }
        val apiKey = dialogEditText("Web API key", settings.firebase.apiKey, false)
        val databaseUrl = dialogEditText("Database URL", settings.firebase.databaseUrl, false)
        val projectId = dialogEditText("Project ID", settings.firebase.projectId, false)
        val workspaceId = dialogEditText("Workspace ID", settings.firebase.workspaceId, false)
        form.addView(apiKey)
        form.addView(databaseUrl)
        form.addView(projectId)
        form.addView(workspaceId)
        AlertDialog.Builder(this)
            .setTitle("Firebase config")
            .setView(form)
            .setPositiveButton("Save") { _, _ ->
                persistSettings(
                    settings.copy(
                        firebase = settings.firebase.copy(
                            enabled = true,
                            realtime = true,
                            apiKey = apiKey.text.toString().trim(),
                            databaseUrl = databaseUrl.text.toString().trim(),
                            projectId = projectId.text.toString().trim(),
                            workspaceId = workspaceId.text.toString().trim(),
                            workspaceName = workspaceId.text.toString().trim(),
                        ),
                    ),
                )
                startFirebaseRealtimeIfEnabled()
                showSync()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun promptFirebaseLogin(register: Boolean) {
        if (!firebaseSyncRepository.backendConfigured(settings.firebase.copy(enabled = true, realtime = true))) {
            setSyncError("Firebase config unavailable. Add bundled defaults or advanced custom config.")
            return
        }
        val form = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(8), dp(4), dp(8), dp(4))
        }
        val email = dialogEditText("Email", settings.firebase.userEmail, false)
        val password = dialogEditText("Password", "", true)
        form.addView(email)
        form.addView(password)
        AlertDialog.Builder(this)
            .setTitle(if (register) "Create Firebase account" else "Firebase login")
            .setView(form)
            .setPositiveButton(if (register) "Create account" else "Login") { _, _ ->
                val next = settings.firebase.copy(
                    enabled = true,
                    realtime = true,
                    userEmail = email.text.toString().trim(),
                )
                persistSettings(settings.copy(firebase = next))
                Thread {
                    runCatching {
                        val session = if (register) {
                            firebaseSyncRepository.register(next, email.text.toString().trim(), password.text.toString())
                        } else {
                            firebaseSyncRepository.login(next, email.text.toString().trim(), password.text.toString())
                        }
                        val workspaceSettings = firebaseSyncRepository.ensurePersonalWorkspace(next, session)
                        Pair(session, workspaceSettings)
                    }.onSuccess { (session, workspaceSettings) ->
                        firebaseSession = session
                        runOnUiThread {
                            persistSettings(settings.copy(firebase = workspaceSettings))
                            setSyncSuccess("Firebase workspace: ${workspaceSettings.workspaceName}")
                            scheduleFirebasePull()
                            pullFirebaseRealtimeData()
                            pushSharedFirebaseData()
                        }
                    }.onFailure { error ->
                        val label = if (register) "account creation" else "login"
                        runOnUiThread { setSyncError("Firebase $label failed: ${error.message}") }
                    }
                }.start()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun loginFirebaseWithGoogle() {
        if (!firebaseSyncRepository.backendConfigured(settings.firebase.copy(enabled = true, realtime = true))) {
            setSyncError("Firebase config unavailable. Add bundled defaults or advanced custom config.")
            return
        }
        if (BuildConfig.GOOGLE_WEB_CLIENT_ID.isBlank()) {
            setSyncError("Google web client ID is not configured.")
            return
        }
        val options = GoogleSignInOptions.Builder(GoogleSignInOptions.DEFAULT_SIGN_IN)
            .requestIdToken(BuildConfig.GOOGLE_WEB_CLIENT_ID)
            .requestEmail()
            .build()
        val client = GoogleSignIn.getClient(this, options)
        setSyncStatus("Opening Google sign-in")
        firebaseSyncRepository.clearSavedSession()
        firebaseSession = null
        client.signOut().addOnCompleteListener {
            startActivityForResult(client.signInIntent, FIREBASE_GOOGLE_SIGN_IN_REQUEST_CODE)
        }
    }

    private fun handleFirebaseGoogleSignInResult(resultCode: Int, data: Intent?) {
        if (data == null) {
            Log.w(TAG, "Google sign-in returned no result data (resultCode=$resultCode)")
            setSyncError("Firebase Google login canceled")
            return
        }
        val account = try {
            GoogleSignIn.getSignedInAccountFromIntent(data).getResult(ApiException::class.java)
        } catch (error: ApiException) {
            Log.e(TAG, "Google sign-in failed with status ${error.statusCode}", error)
            setSyncError(formatGoogleSignInError(error.statusCode, error.message))
            return
        }
        val googleIdToken = account.idToken
        if (googleIdToken.isNullOrBlank()) {
            setSyncError("Firebase Google login failed: missing Google ID token")
            return
        }
        val next = settings.firebase.copy(
            enabled = true,
            realtime = true,
            userEmail = account.email.orEmpty(),
        )
        persistSettings(settings.copy(firebase = next))
        Thread {
            runCatching {
                val session = firebaseSyncRepository.loginWithGoogleIdToken(next, googleIdToken)
                val workspaceSettings = firebaseSyncRepository.ensurePersonalWorkspace(
                    next.copy(userEmail = session.email.ifBlank { next.userEmail }),
                    session,
                )
                Pair(session, workspaceSettings)
            }.onSuccess { (session, workspaceSettings) ->
                firebaseSession = session
                runOnUiThread {
                    persistSettings(settings.copy(firebase = workspaceSettings))
                    setSyncSuccess("Firebase Google login ready: ${workspaceSettings.workspaceName}")
                    scheduleFirebasePull()
                    pullFirebaseRealtimeData()
                    pushSharedFirebaseData()
                }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase Google login failed: ${error.message}") }
            }
        }.start()
    }

    private fun dialogEditText(hintText: String, value: String, password: Boolean): EditText {
        return EditText(this).apply {
            hint = hintText
            setText(value)
            inputType = if (password) {
                InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
            } else {
                InputType.TYPE_CLASS_TEXT
            }
            setSingleLine(true)
        }
    }

    private fun pushTodosToFirebase() {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                firebaseSyncRepository.pushTodos(settings.firebase, todoRepository.load(), session)
            }.onSuccess {
                runOnUiThread { setSyncSuccess("Firebase todos pushed") }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase push failed: ${error.message}") }
            }
        }.start()
    }

    private fun pushNoteToFirebase(path: String, text: String) {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                firebaseSyncRepository.pushNote(settings.firebase, path, text, session)
            }.onSuccess {
                runOnUiThread { setSyncSuccess("Firebase note pushed") }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase note push failed: ${error.message}") }
            }
        }.start()
    }

    private fun pushNoteDeleteToFirebase(path: String) {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                firebaseSyncRepository.pushNoteDelete(settings.firebase, path, session)
            }.onSuccess {
                runOnUiThread { setSyncSuccess("Firebase note deleted") }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase note delete failed: ${error.message}") }
            }
        }.start()
    }

    private fun scheduleSharedSettingsPush() {
        firebaseSettingsPushRunnable?.let { noteAutosaveHandler.removeCallbacks(it) }
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        val runnable = Runnable {
            firebaseSettingsPushRunnable = null
            pushSharedFirebaseData(pushSettings = true)
        }
        firebaseSettingsPushRunnable = runnable
        noteAutosaveHandler.postDelayed(runnable, 1_000L)
    }

    private fun pushSharedFirebaseData(pushSettings: Boolean = true) {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                val settingsPushed = if (pushSettings) firebaseSyncRepository.pushSharedSettings(settings.firebase, settings, session) else false
                firebaseSyncRepository.deleteLegacyAssetsBestEffort(settings.firebase, session)
                settingsPushed
            }.onSuccess { settingsPushed ->
                runOnUiThread {
                    val verb = if (pushSettings && settingsPushed) "pushed" else "unchanged"
                    setSyncSuccess("Firebase shared settings $verb")
                }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase shared push failed: ${error.message}") }
            }
        }.start()
    }

    private fun pullSharedFirebaseData() {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                firebaseSyncRepository.deleteLegacyAssetsBestEffort(settings.firebase, session)
                firebaseSyncRepository.pullSharedSettings(settings.firebase, session)
            }.onSuccess { shared ->
                runOnUiThread {
                    val localEditActive = hasActiveLocalEdit()
                    if (shared != null && localEditActive) {
                        setSyncStatus("Firebase shared settings deferred while local edits are active")
                    }
                    if (shared != null && shared.rev <= pagesLocalEditAtMs) {
                        setSyncStatus("Firebase shared settings skipped because local Pages are newer")
                    }
                    if (shared != null && !localEditActive && shared.rev > pagesLocalEditAtMs) {
                        val next = SettingsRepository.applySharedSettings(settings, shared.values)
                        settings = next
                        settingsRepository.save(settings)
                        firebaseSyncRepository.markSharedSettingsSynced(settings.firebase, shared.values)
                    }
                    if (!localEditActive) {
                        when (currentScreen) {
                            Screen.Pages -> showPages()
                            Screen.Settings -> showSettings()
                            Screen.Notes -> {
                                noteList = notesRepository.listNotes()
                                updateNoteSelector()
                            }
                            else -> Unit
                        }
                    }
                    if (!localEditActive) {
                        val settingsStatus = if (shared != null) "shared settings pulled, " else ""
                        setSyncSuccess("Firebase shared sync: ${settingsStatus}complete")
                    }
                }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase shared pull failed: ${error.message}") }
            }
        }.start()
    }

    private fun pullFirebaseRealtimeData() {
        pullTodosFromFirebase()
    }

    private fun pullTodosFromFirebase() {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                val local = todoRepository.load()
                val merged = firebaseSyncRepository.pullTodos(settings.firebase, local, session)
                val todoChanged = merged != local
                if (todoChanged) {
                    todoRepository.save(merged)
                }
                val remoteNotes = firebaseSyncRepository.pullNotes(settings.firebase, session)
                val shared = firebaseSyncRepository.pullSharedSettings(settings.firebase, session)
                firebaseSyncRepository.deleteLegacyAssetsBestEffort(settings.firebase, session)
                FirebasePullResult(
                    todos = merged,
                    todoChanged = todoChanged,
                    remoteNotes = remoteNotes,
                    remoteTodoCount = merged.items.size,
                    remoteNoteCount = remoteNotes.count { !it.deleted },
                ) to shared
            }.onSuccess { result ->
                runOnUiThread {
                    val pullResult = result.first
                    val shared = result.second
                    val localEditActive = hasActiveLocalEdit()
                    if (shared != null && localEditActive) {
                        setSyncStatus("Firebase shared settings deferred while local edits are active")
                    }
                    if (shared != null && shared.rev <= pagesLocalEditAtMs) {
                        setSyncStatus("Firebase shared settings skipped because local Pages are newer")
                    }
                    if (shared != null && !localEditActive && shared.rev > pagesLocalEditAtMs) {
                        settings = SettingsRepository.applySharedSettings(settings, shared.values)
                        settingsRepository.save(settings)
                        firebaseSyncRepository.markSharedSettingsSynced(settings.firebase, shared.values)
                    }
                    applyRemoteNotes(pullResult.remoteNotes)
                    todoStore = pullResult.todos
                    if (SyncUiState.shouldRebuildTodoAfterPull(
                            todoChanged = pullResult.todoChanged,
                            showingTodo = currentScreen == Screen.Todo,
                            canRebuild = canRebuildTodoAfterRemotePull(),
                        )
                    ) {
                        showTodoPreservingScroll()
                    }
                    if (currentScreen == Screen.Pages && !localEditActive) showPages()
                    val deferredNoteCount = pendingRemoteNotes.size
                    val sharedStatus = if (shared != null) ", shared settings pulled" else ""
                    val deferredStatus = if (deferredNoteCount > 0) ", $deferredNoteCount note(s) deferred for local edits" else ""
                    setSyncSuccess(
                        "Firebase sync: ${pullResult.remoteTodoCount} todo(s), ${pullResult.remoteNoteCount} note(s)$sharedStatus$deferredStatus in ${settings.firebase.workspaceId}",
                    )
                }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase pull failed: ${error.message}") }
            }
        }.start()
    }

	private fun syncToFirebase() {
		if (!firebaseSyncRepository.configured(settings.firebase)) return
		setSyncStatus("Firebase sync started", transient = true)
        saveCurrentNoteSilently()
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                val localTodos = todoRepository.load()
                val mergedTodos = firebaseSyncRepository.pullTodos(settings.firebase, localTodos, session, forceFull = true)
                val todoChanged = mergedTodos != localTodos
                if (todoChanged) {
                    todoRepository.save(mergedTodos)
                }
                val remoteNotes = firebaseSyncRepository.pullNotes(settings.firebase, session)
                val shared = firebaseSyncRepository.pullSharedSettings(settings.firebase, session)
                firebaseSyncRepository.deleteLegacyAssetsBestEffort(settings.firebase, session)
                Pair(
                    FirebasePullResult(
                        todos = mergedTodos,
                        todoChanged = todoChanged,
                        remoteNotes = remoteNotes,
                        remoteTodoCount = mergedTodos.items.size,
                        remoteNoteCount = remoteNotes.count { !it.deleted },
                    ),
                    shared,
                )
            }.onSuccess { result ->
                runOnUiThread {
                    val pullResult = result.first
                    val shared = result.second
                    val localEditActive = hasActiveLocalEdit()
                    if (shared != null && localEditActive) {
                        setSyncStatus("Firebase shared settings deferred while local edits are active")
                    }
                    if (shared != null && shared.rev <= pagesLocalEditAtMs) {
                        setSyncStatus("Firebase shared settings skipped because local Pages are newer")
                    }
                    if (shared != null && !localEditActive && shared.rev > pagesLocalEditAtMs) {
                        settings = SettingsRepository.applySharedSettings(settings, shared.values)
                        settingsRepository.save(settings)
                        firebaseSyncRepository.markSharedSettingsSynced(settings.firebase, shared.values)
                    }
                    applyRemoteNotes(pullResult.remoteNotes)
                    todoStore = pullResult.todos
                    if (SyncUiState.shouldRebuildTodoAfterPull(
                            todoChanged = pullResult.todoChanged,
                            showingTodo = currentScreen == Screen.Todo,
                            canRebuild = canRebuildTodoAfterRemotePull(),
                        )
                    ) {
                        showTodoPreservingScroll()
                    }
                    if (currentScreen == Screen.Pages && !localEditActive) showPages()
                    pushLocalStateAfterManualFirebasePull(pullResult, shared)
                }
			}.onFailure { error ->
				runOnUiThread { setSyncError("Firebase sync failed: ${error.message}", transient = true) }
			}
		}.start()
	}

    private fun pushLocalStateAfterManualFirebasePull(
        pullResult: FirebasePullResult,
        shared: FirebaseSharedSettings?,
    ) {
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                firebaseSyncRepository.pushTodos(settings.firebase, todoRepository.load(), session)
                val settingsPushed = firebaseSyncRepository.pushSharedSettings(settings.firebase, settings, session)
                firebaseSyncRepository.deleteLegacyAssetsBestEffort(settings.firebase, session)
                settingsPushed
            }.onSuccess { settingsPushed ->
                runOnUiThread {
                    val deferredNoteCount = pendingRemoteNotes.size
                    val sharedStatus = if (shared != null || settingsPushed) ", shared settings synced" else ""
                    val deferredStatus = if (deferredNoteCount > 0) ", $deferredNoteCount note(s) deferred for local edits" else ""
					setSyncSuccess(
						"Firebase sync: ${pullResult.remoteTodoCount} todo(s), ${pullResult.remoteNoteCount} note(s)$sharedStatus$deferredStatus in ${settings.firebase.workspaceId}",
						transient = true,
					)
				}
			}.onFailure { error ->
				runOnUiThread { setSyncError("Firebase sync failed: ${error.message}", transient = true) }
			}
		}.start()
	}

    private fun replaceLocalFromFirebase() {
        if (!firebaseSyncRepository.configured(settings.firebase)) return
        if (hasActiveLocalEdit()) {
            setSyncStatus("Firebase replace deferred while local edits are active")
            return
        }
        Thread {
            val session = freshFirebaseSession()
            if (session == null) {
                runOnUiThread { setSyncError("Firebase login required") }
                return@Thread
            }
            runCatching {
                val remoteTodos = firebaseSyncRepository.pullRemoteTodoStore(settings.firebase, session)
                val remoteNotes = firebaseSyncRepository.pullNotes(settings.firebase, session)
                val shared = firebaseSyncRepository.pullSharedSettings(settings.firebase, session)
                firebaseSyncRepository.deleteLegacyAssetsBestEffort(settings.firebase, session)
                Pair(Pair(remoteTodos, remoteNotes), shared)
            }.onSuccess { result ->
                runOnUiThread {
                    val remoteTodos = result.first.first
                    val remoteNotes = result.first.second
                    val shared = result.second
                    todoRepository.save(remoteTodos)
                    todoStore = remoteTodos
                    notesRepository.clearAll()
                    shared?.let {
                        settings = SettingsRepository.applySharedSettings(settings, it.values)
                        settingsRepository.save(settings)
                        firebaseSyncRepository.markSharedSettingsSynced(settings.firebase, it.values)
                    }
                    applyRemoteNotes(remoteNotes)
                    if (currentScreen == Screen.Todo && canRebuildTodoAfterRemotePull()) {
                        showTodo()
                    }
                    if (currentScreen == Screen.Notes) {
                        refreshNotes()
                        if (currentNotePath.isNotBlank()) {
                            selectNote(currentNotePath)
                        }
                    }
                    if (currentScreen == Screen.Pages) showPages()
                    setSyncSuccess(
                        "Firebase replaced local data: ${remoteTodos.items.size} todo(s), ${remoteNotes.count { !it.deleted }} note(s)",
                    )
                }
            }.onFailure { error ->
                runOnUiThread { setSyncError("Firebase replace failed: ${error.message}") }
            }
        }.start()
    }

    private fun hasActiveLocalEdit(): Boolean {
        val pageInputActive = currentScreen == Screen.Pages && System.currentTimeMillis() < pagesLocalEditUntilMs
        val noteEditFocused = currentScreen == Screen.Notes &&
            (noteEditor?.hasFocus() == true || rawNoteEditor?.hasFocus() == true || pendingNoteAutosave != null)
        val todoEditActive = currentScreen == Screen.Todo && todoDraftText.isNotBlank()
        return pageInputActive || noteEditFocused || todoEditActive
    }

    private fun applyRemoteNotes(remoteNotes: List<FirebaseRemoteNote>) {
        var noteFilesChanged = false
        remoteNotes.forEach { remote ->
            val path = NotesRepository.normalizePath(remote.path)
            val isCurrent = path == currentNotePath
            if (shouldDeferRemoteNote(path)) {
                pendingRemoteNotes[path] = remote
                return@forEach
            }
            if (remote.deleted) {
                if (isCurrent) {
                    clearCurrentNoteFromRemoteDelete()
                }
                notesRepository.delete(path)
                noteFilesChanged = true
                return@forEach
            }
            val localText = notesRepository.read(path)
            if (localText != remote.text) {
                notesRepository.save(path, remote.text)
                noteFilesChanged = true
            }
            if (isCurrent) {
                applyRemoteTextToCurrentNote(remote.text)
            }
        }
        if (noteFilesChanged) {
            noteList = notesRepository.listNotes()
        }
        updateNoteSelector()
    }

    private fun shouldDeferRemoteNote(path: String): Boolean {
        return currentScreen == Screen.Notes &&
            path == currentNotePath &&
            (hasUnsavedNoteChanges() || pendingNoteAutosave != null)
    }

    private fun applyPendingRemoteNoteToFile(path: String, savedText: String, updateEditor: Boolean) {
        val remote = pendingRemoteNotes.remove(path) ?: return
        if (remote.deleted) {
            if (savedText.isNotBlank()) {
                val conflictPath = conflictNotePath(path)
                notesRepository.save(conflictPath, savedText)
                pushNoteToFirebase(conflictPath, savedText)
            }
            notesRepository.delete(path)
            if (updateEditor) {
                clearCurrentNoteFromRemoteDelete()
            }
            return
        }
        if (remote.text != savedText) {
            val conflictPath = conflictNotePath(path)
            notesRepository.save(conflictPath, savedText)
            pushNoteToFirebase(conflictPath, savedText)
            notesRepository.save(path, remote.text)
            if (updateEditor) {
                applyRemoteTextToCurrentNote(remote.text)
            }
        }
    }

    private fun conflictNotePath(path: String): String {
        val normalized = NotesRepository.normalizePath(path)
        val dot = normalized.lastIndexOf('.')
        val stem = if (dot >= 0) normalized.substring(0, dot) else normalized
        val ext = if (dot >= 0) normalized.substring(dot) else ".md"
        return "$stem.conflict-android-${System.currentTimeMillis()}$ext"
    }

    private fun canRebuildTodoAfterRemotePull(): Boolean {
        val input = currentFocus as? EditText
        if (input?.id == R.id.todo_input) return false
        return todoDraftText.isBlank()
    }

    private fun setSyncSuccess(status: String, transient: Boolean = false) =
        setSyncStatus(status, transient, SyncOutcome.Success)

    private fun setSyncError(status: String, transient: Boolean = false) =
        setSyncStatus(status, transient, SyncOutcome.Error)

    private fun setSyncStatus(
        status: String,
        transient: Boolean = false,
        outcome: SyncOutcome = SyncOutcome.Information,
    ) {
        lastSyncStatus = status
        syncStatusText?.text = status
        if (outcome != SyncOutcome.Information) {
            val firebaseStatus = if (outcome == SyncOutcome.Success) {
                FirebaseSyncStatus.Success
            } else {
                FirebaseSyncStatus.Error
            }
            settings = settings.copy(
                firebase = settings.firebase.copy(
                    lastSyncAt = syncTimestamp(),
                    lastSyncStatus = firebaseStatus,
                    lastSyncMessage = status,
                ),
            )
            settingsRepository.save(settings)
        }
        updateSyncProblemIndicator()
        if (transient && currentScreen != Screen.Sync) {
            Toast.makeText(this, status, Toast.LENGTH_SHORT).show()
        }
    }

	private fun currentSyncStatus(): String {
		if (lastSyncStatus.isNotBlank()) return lastSyncStatus
		if (settings.firebase.lastSyncMessage.isNotBlank()) return settings.firebase.lastSyncMessage
		if (settings.firebase.enabled) {
            val workspace = settings.firebase.workspaceName.ifBlank { settings.firebase.workspaceId }
            return if (firebaseSyncRepository.configured(settings.firebase)) {
                "Firebase realtime: ${workspace.ifBlank { "configured" }} (${settings.firebase.workspaceId})"
            } else if (firebaseSyncRepository.backendConfigured(settings.firebase)) {
                if (firebaseSyncRepository.hasSavedSession()) {
                    "Firebase: ready, workspace setup pending"
                } else {
                    "Firebase: ready. Create account or login."
                }
            } else {
                "Firebase config unavailable. Add bundled defaults or advanced custom config."
            }
        }
        if (FirebaseDefaults.bundled.ready) return "Firebase: ready. Create account or login."
        return "Firebase config unavailable. Add bundled defaults or advanced custom config."
    }

    private fun firebaseHasProblem(): Boolean {
        return SyncUiState.hasFirebaseProblem(
            firebase = settings.firebase,
            backendConfigured = firebaseSyncRepository.backendConfigured(settings.firebase),
            hasSavedSession = firebaseSyncRepository.hasSavedSession(),
        )
    }

    private fun updateSyncProblemIndicator() {
        if (!::syncProblemBadge.isInitialized) return
        val hasProblem = firebaseHasProblem()
        syncProblemBadge.visibility = if (hasProblem) View.VISIBLE else View.GONE
        syncToolbarControl.contentDescription = if (hasProblem) {
            "Firebase sync needs attention. Open Sync"
        } else {
            "Quick sync to Firebase"
        }
    }

    private fun syncTimestamp(): String {
        return SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US).apply {
            timeZone = TimeZone.getTimeZone("UTC")
        }.format(Date())
    }

    private fun dialogButton(label: String): Button {
        return Button(this).apply {
            text = label
            textSize = 13f
            transformationMethod = null
            setTextColor(COLOR_ACCENT)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(8).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT,
                dp(42),
            ).apply {
                leftMargin = dp(6)
            }
        }
    }

    private fun dialogIconButton(icon: Int, id: Int, description: String): ImageButton {
        return ImageButton(this).apply {
            this.id = id
            contentDescription = description
            setImageResource(icon)
            setColorFilter(COLOR_ACCENT)
            scaleType = ImageView.ScaleType.CENTER
            setPadding(dp(9), dp(9), dp(9), dp(9))
            background = selectableBorderlessBackground()
            layoutParams = LinearLayout.LayoutParams(dp(44), dp(44)).apply {
                leftMargin = dp(6)
            }
        }
    }

    private fun setScreenHeader(title: String, subtitle: String) {
        titleText.text = title
        subtitleText.text = subtitle
    }

    private fun sectionTitle(text: String): TextView {
        return TextView(this).apply {
            this.text = text
            textSize = 13f
            setTypeface(Typeface.DEFAULT, Typeface.BOLD)
            setTextColor(COLOR_TEXT_SECONDARY)
            setPadding(0, dp(12), 0, dp(6))
        }
    }

    private fun formRow(label: String, input: EditText?): LinearLayout {
        return LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            ).apply {
                bottomMargin = dp(10)
            }
            addView(TextView(this@MainActivity).apply {
                text = label
                textSize = 13f
                setTextColor(COLOR_TEXT_SECONDARY)
                setPadding(dp(2), 0, 0, dp(4))
            })
            input?.let { addView(it) }
        }
    }

    private fun resultPanel(): TextView {
        return TextView(this).apply {
            id = R.id.pages_result
            textSize = 24f
            setTypeface(Typeface.DEFAULT, Typeface.BOLD)
            setTextColor(COLOR_TEXT_PRIMARY)
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(16), 0, dp(16), 0)
            background = roundedFill(COLOR_RESULT, dp(12).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(72),
            ).apply {
                topMargin = dp(10)
            }
        }
    }

    private fun commandButton(label: String, id: Int, action: () -> Unit): Button {
        return Button(this).apply {
            text = label
            this.id = id
            textSize = 15f
            transformationMethod = null
            setTextColor(COLOR_ACCENT)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(10).toFloat())
            setOnClickListener { action() }
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(46),
            ).apply {
                bottomMargin = dp(8)
            }
        }
    }

    private fun roundedFill(color: Int, radius: Float): GradientDrawable {
        return GradientDrawable().apply {
            setColor(color)
            cornerRadius = radius
        }
    }

    private fun roundedStroke(fill: Int, stroke: Int, radius: Float): GradientDrawable {
        return GradientDrawable().apply {
            setColor(fill)
            setStroke(dp(1), stroke)
            cornerRadius = radius
        }
    }

    private fun selectableBorderlessBackground() = selectableBackground(android.R.attr.selectableItemBackgroundBorderless)

    private fun selectableFillBackground(color: Int) = selectableBackground(android.R.attr.selectableItemBackground).apply {
        setTint(color)
    }

    private fun selectableBackground(attribute: Int): android.graphics.drawable.Drawable {
        val value = TypedValue()
        theme.resolveAttribute(attribute, value, true)
        return getDrawable(value.resourceId)!!
    }

    private fun dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }

    private fun resolvePalette(): AppPalette {
        return when (settings.androidApp.themeMode) {
            ThemeMode.Light -> AppPalette.light()
            ThemeMode.Dark -> AppPalette.dark()
            ThemeMode.System -> if (isSystemDarkTheme()) AppPalette.dark() else AppPalette.light()
        }
    }

    private fun isSystemDarkTheme(): Boolean {
        val mode = resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK
        return mode == Configuration.UI_MODE_NIGHT_YES
    }

    private fun applySystemBars() {
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightStatusBars = palette.lightSystemBars
            isAppearanceLightNavigationBars = palette.lightSystemBars
        }
    }

    private val COLOR_APP_BACKGROUND: Int get() = palette.appBackground
    private val COLOR_TOOLBAR: Int get() = palette.toolbar
    private val COLOR_DRAWER: Int get() = palette.drawer
    private val COLOR_SURFACE: Int get() = palette.surface
    private val COLOR_SURFACE_ALT: Int get() = palette.surfaceAlt
    private val COLOR_BORDER: Int get() = palette.border
    private val COLOR_STATUS: Int get() = palette.status
    private val COLOR_RESULT: Int get() = palette.result
    private val COLOR_ACCENT: Int get() = palette.accent
    private val COLOR_TEXT_PRIMARY: Int get() = palette.textPrimary
    private val COLOR_TEXT_SECONDARY: Int get() = palette.textSecondary
    private val COLOR_TEXT_MUTED: Int get() = palette.textMuted
    private val COLOR_SCRIM: Int get() = palette.scrim

    private enum class Screen(val settingValue: String, val syncScope: FirebaseViewSyncScope) {
        Notes(AndroidScreenState.NOTES, FirebaseViewSyncScope.Notes),
        Pages(AndroidScreenState.PAGES, FirebaseViewSyncScope.Settings),
        Todo(AndroidScreenState.TODO, FirebaseViewSyncScope.Todos),
        Sync(AndroidScreenState.SYNC, FirebaseViewSyncScope.Full),
        Settings(AndroidScreenState.SETTINGS, FirebaseViewSyncScope.Settings),
    }

    private enum class FirebaseViewSyncScope(val label: String) {
        Todos("todo"),
        Notes("notes"),
        Settings("settings"),
        Full("full"),
    }

    private enum class SyncOutcome {
        Information,
        Success,
        Error,
    }

    companion object {
        private const val TAG = "MainActivity"
        private const val FIREBASE_GOOGLE_SIGN_IN_REQUEST_CODE = 4203
        private const val DRAWER_ANIMATION_MS = 180L
        private const val NOTE_AUTOSAVE_DELAY_MS = 600L
        private const val FIREBASE_PULL_INTERVAL_MS = 30 * 1_000L
        private const val PRIVACY_POLICY_URL = "https://koko.lv/privacy-policy.html"
        private const val ACCOUNT_DELETION_URL = "https://koko.lv/account-deletion.html"
    }
}

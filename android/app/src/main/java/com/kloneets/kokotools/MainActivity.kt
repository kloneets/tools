package com.kloneets.kokotools

import android.app.Activity
import android.app.AlertDialog
import android.content.res.ColorStateList
import android.content.res.Configuration
import android.content.Intent
import android.content.IntentSender
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.InputType
import android.text.TextWatcher
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.BaseAdapter
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.PopupMenu
import android.widget.RadioButton
import android.widget.RadioGroup
import android.widget.TextView
import android.widget.Toast
import com.google.android.gms.auth.api.identity.AuthorizationRequest
import com.google.android.gms.auth.api.identity.AuthorizationResult
import com.google.android.gms.auth.api.identity.Identity
import com.google.android.gms.common.api.ApiException
import com.google.android.gms.common.api.Scope
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailability
import java.io.IOException

class MainActivity : Activity() {
    private lateinit var settingsRepository: SettingsRepository
    private lateinit var notesRepository: NotesRepository
    private val pagesCalculator = PagesCalculator()
    private val driveRepository = DriveSnapshotRepository()

    private var settings = AppSettings()
    private var currentNotePath = ""
    private var noteList: List<NoteFile> = emptyList()
    private var driveAccessToken: String? = null
    private var driveAuthInProgress = false
    private var selectedSnapshotId = ""
    private var currentScreen = Screen.Notes
    private var palette = AppPalette.light()

    private lateinit var root: FrameLayout
    private lateinit var appLayout: LinearLayout
    private lateinit var titleText: TextView
    private lateinit var subtitleText: TextView
    private lateinit var actionsButton: ImageButton
    private lateinit var drawerScrim: View
    private lateinit var drawerPanel: LinearLayout
    private lateinit var content: LinearLayout

    private var noteListView: ListView? = null
    private var noteSelector: TextView? = null
    private var noteEditor: HybridMarkdownEditor? = null
    private var rawNoteEditor: EditText? = null
    private var loadedNoteText = ""
    private val noteAutosaveHandler = Handler(Looper.getMainLooper())
    private var pendingNoteAutosave: Runnable? = null
    private var suppressNoteAutosave = false
    private val notePickerExpandedFolders = mutableSetOf<String>()

    private var firstInput: EditText? = null
    private var readInput: EditText? = null
    private var secondInput: EditText? = null
    private var resultText: TextView? = null

    private var syncStatusText: TextView? = null
    private var syncFolderTitle: TextView? = null
    private var syncActionTitle: TextView? = null
    private var syncSnapshotTitle: TextView? = null
    private var syncFolderText: TextView? = null
    private var syncSelectFolderButton: Button? = null
    private var syncSnapshotList: ListView? = null
    private var syncConnectButton: Button? = null
    private var syncUploadButton: Button? = null
    private var syncRefreshButton: Button? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settingsRepository = SettingsRepository(this)
        notesRepository = NotesRepository(this)
        settings = settingsRepository.load()
        palette = resolvePalette()
        applySystemBars()
        currentNotePath = settings.notesApp.currentNotePath
        selectedSnapshotId = settings.gdrive.selectedSnapshotId
        buildRoot()
        showNotes()
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != DRIVE_AUTH_REQUEST_CODE) return
        driveAuthInProgress = false
        updateSyncButtons()
        if (resultCode != RESULT_OK || data == null) {
            setSyncStatus("Google authorization canceled. If the Google screen flashed, add a Google account in the emulator first.")
            return
        }
        try {
            handleAuthorizationResult(Identity.getAuthorizationClient(this).getAuthorizationResultFromIntent(data))
        } catch (error: ApiException) {
            setSyncStatus("Google authorization failed: ${error.message}")
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
        toolbar.addView(toolsButton)
        toolbar.addView(titleGroup)
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
            Screen.Sync -> showSyncActionsMenu(actionsButton)
            Screen.Settings -> showSettingsActionsMenu(actionsButton)
        }
    }

    private fun showNotes() {
        currentScreen = Screen.Notes
        setScreenHeader("Notes", currentNoteLabel())
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(0, 0, 0, 0)

        noteListView = null
        noteSelector = null
        rawNoteEditor = null

        if (settings.notesApp.previewHidden) {
            noteEditor = null
            rawNoteEditor = buildRawNoteEditor()
            content.addView(rawNoteEditor)
        } else {
            noteEditor = HybridMarkdownEditor(this, palette).apply {
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
            inputType = InputType.TYPE_CLASS_TEXT or
                InputType.TYPE_TEXT_FLAG_MULTI_LINE or
                InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
            setTextColor(COLOR_TEXT_PRIMARY)
            setHintTextColor(COLOR_TEXT_MUTED)
            setPadding(dp(14), dp(12), dp(14), dp(12))
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
                }
            })
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

    private fun toggleNoteRendering() {
        saveCurrentNoteSilently()
        persistSettings(
            settings.copy(
                notesApp = settings.notesApp.copy(previewHidden = !settings.notesApp.previewHidden),
            ),
        )
        showNotes()
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
        saveCurrentNoteSilently()
        selectNote(relativePath)
        afterSwitch()
    }

    private fun hasUnsavedNoteChanges(): Boolean {
        return noteEditorText() != loadedNoteText
    }

    private fun promptNewNote() {
        val input = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_TEXT
            hint = "folder/name.md"
        }
        AlertDialog.Builder(this)
            .setTitle("New note")
            .setView(input)
            .setPositiveButton("Create") { _, _ ->
                val path = NotesRepository.normalizePath(input.text.toString())
                currentNotePath = path
                notesRepository.save(path, "")
                loadedNoteText = ""
                refreshNotes()
                selectNote(path)
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
        val saved = notesRepository.save(path, noteEditorText())
        currentNotePath = saved.relativePath
        persistSettings(
            settings.copy(notesApp = settings.notesApp.copy(currentNotePath = saved.relativePath)),
        )
        loadedNoteText = noteEditorText()
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
                notesRepository.delete(path)
                currentNotePath = ""
                loadedNoteText = ""
                persistSettings(settings.copy(notesApp = settings.notesApp.copy(currentNotePath = "")))
                refreshNotes()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun showPages() {
        currentScreen = Screen.Pages
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
                recalculatePages()
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
                recalculatePages()
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

    private fun recalculatePages() {
        val result = pagesCalculator.calculate(
            readInput?.text?.toString().orEmpty(),
            firstInput?.text?.toString().orEmpty(),
            secondInput?.text?.toString().orEmpty(),
        )
        resultText?.text = result.label
        persistSettings(
            settings.copy(
                pagesApp = PagesSettings(
                    firstBook = result.firstBookPages,
                    secondBook = result.secondBookPages,
                    readPages = result.readPages,
                ),
            ),
        )
    }

    private fun showSync() {
        currentScreen = Screen.Sync
        setScreenHeader("Sync", "Google Drive snapshots")
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
        content.addView(TextView(this).apply {
            text = "Emulator requirement: use a Google Play system image, sign into a Google account, and complete any Google screen-lock prompt before connecting."
            textSize = 13f
            setTextColor(COLOR_TEXT_SECONDARY)
            setPadding(dp(4), dp(8), dp(4), dp(4))
        })

        syncFolderTitle = sectionTitle("Drive folder")
        content.addView(syncFolderTitle)
        syncFolderText = TextView(this).apply {
            id = R.id.sync_folder_id
            textSize = 15f
            setTextColor(COLOR_TEXT_PRIMARY)
            setPadding(dp(14), dp(12), dp(14), dp(12))
            background = roundedFill(COLOR_SURFACE, dp(10).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            )
        }
        content.addView(syncFolderText)

        syncSelectFolderButton = commandButton("Select Drive folder", R.id.sync_select_folder) {
            showDriveFolderPicker()
        }
        content.addView(syncSelectFolderButton)

        syncActionTitle = sectionTitle("Snapshot actions")
        content.addView(syncActionTitle)
        syncConnectButton = commandButton("Connect Google", R.id.sync_connect) {
            connectGoogleDrive()
        }
        syncUploadButton = commandButton("Upload snapshot", R.id.sync_upload) {
            uploadDriveSnapshot()
        }
        syncRefreshButton = commandButton("Refresh snapshots", R.id.sync_refresh) {
            refreshDriveSnapshots()
        }
        content.addView(syncConnectButton)
        content.addView(syncUploadButton)
        content.addView(syncRefreshButton)

        syncSnapshotTitle = sectionTitle("Snapshots")
        content.addView(syncSnapshotTitle)
        syncSnapshotList = ListView(this).apply {
            id = R.id.sync_snapshot_list
            divider = null
            background = roundedFill(COLOR_SURFACE, dp(10).toFloat())
            setPadding(0, dp(4), 0, dp(4))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                1f,
            )
        }
        content.addView(syncSnapshotList)

        refreshSnapshotListView()
        updateSyncButtons()
    }

    private fun showSyncActionsMenu(anchor: View) {
        val visibility = currentSyncActionVisibility()
        PopupMenu(this, anchor).apply {
            if (visibility.connect) menu.add("Connect Google").setOnMenuItemClickListener {
                connectGoogleDrive()
                true
            }
            if (visibility.folderSelection) menu.add("Select Drive folder").setOnMenuItemClickListener {
                showDriveFolderPicker()
                true
            }
            if (visibility.upload) menu.add("Upload snapshot").setOnMenuItemClickListener {
                uploadDriveSnapshot()
                true
            }
            if (visibility.refresh) menu.add("Refresh snapshots").setOnMenuItemClickListener {
                refreshDriveSnapshots()
                true
            }
            show()
        }
    }

    private fun showSettings() {
        currentScreen = Screen.Settings
        setScreenHeader("Settings", "App preferences")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
        content.setPadding(dp(16), dp(16), dp(16), dp(16))

        content.addView(sectionTitle("Theme"))
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
        content.addView(group)
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
        when (currentScreen) {
            Screen.Notes -> showNotes()
            Screen.Pages -> showPages()
            Screen.Sync -> showSync()
            Screen.Settings -> showSettings()
        }
    }

    private fun persistSettings(next: AppSettings) {
        settings = next
        settingsRepository.save(settings)
    }

    private fun saveSelectedDriveFolder(folder: DriveSnapshotRepository.DriveEntry) {
        selectedSnapshotId = ""
        persistSettings(
            settings.copy(
                gdrive = settings.gdrive.copy(
                    folderId = folder.id,
                    folderName = folder.name,
                    selectedSnapshotId = "",
                    snapshots = emptyList(),
                ),
            ),
        )
        refreshSnapshotListView()
        syncFolderText?.text = selectedFolderLabel()
        setSyncStatus("Synced to ${folder.name}")
        updateSyncButtons()
        if (driveAccessToken != null) {
            refreshDriveSnapshots()
        }
    }

    private fun connectGoogleDrive() {
        if (driveAuthInProgress) {
            setSyncStatus("Google authorization is already open")
            return
        }
        val availability = GoogleApiAvailability.getInstance()
        val status = availability.isGooglePlayServicesAvailable(this)
        if (status != ConnectionResult.SUCCESS) {
            val message = availability.getErrorString(status)
            setSyncStatus("Google Play services unavailable: $message")
            availability.getErrorDialog(this, status, PLAY_SERVICES_REQUEST_CODE)?.show()
            return
        }

        setSyncStatus("Opening Google authorization")
        driveAuthInProgress = true
        updateSyncButtons()
        val request = AuthorizationRequest.builder()
            .setRequestedScopes(listOf(Scope(DriveSnapshotRepository.DRIVE_SCOPE)))
            .build()
        Identity.getAuthorizationClient(this).authorize(request)
            .addOnSuccessListener { handleAuthorizationResult(it) }
            .addOnFailureListener {
                driveAuthInProgress = false
                setSyncStatus("Google authorization failed: ${it.message ?: it.javaClass.simpleName}")
                updateSyncButtons()
            }
    }

    private fun handleAuthorizationResult(result: AuthorizationResult) {
        if (result.hasResolution()) {
            val pendingIntent = result.pendingIntent
            if (pendingIntent == null) {
                driveAuthInProgress = false
                setSyncStatus("Google authorization needs resolution but did not return an intent")
                updateSyncButtons()
                return
            }
            try {
                startIntentSenderForResult(
                    pendingIntent.intentSender,
                    DRIVE_AUTH_REQUEST_CODE,
                    null,
                    0,
                    0,
                    0,
                )
            } catch (error: IntentSender.SendIntentException) {
                driveAuthInProgress = false
                setSyncStatus("Google authorization failed: ${error.message}")
                updateSyncButtons()
            }
            return
        }
        val token = result.accessToken
        if (token.isNullOrBlank()) {
            driveAuthInProgress = false
            setSyncStatus("Google authorization did not return a Drive access token")
            updateSyncButtons()
            return
        }
        driveAuthInProgress = false
        driveAccessToken = token
        setSyncStatus("Google Drive connected")
        updateSyncButtons()
        if (settings.gdrive.folderId.isNotBlank()) {
            refreshDriveSnapshots()
        }
    }

    private fun uploadDriveSnapshot() {
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        val folderId = settings.gdrive.folderId
        if (folderId.isBlank()) return setSyncStatus("Select a Drive folder first")

        runDriveOperation("Uploading snapshot") {
            settingsRepository.save(settings)
            val snapshot = driveRepository.uploadSnapshot(
                folderId = folderId,
                accessToken = token,
                settingsData = settingsRepository.settingsPath().readBytes(),
                notesRoot = notesRepository.notesPath(),
                retain = 5,
            )
            val snapshots = driveRepository.listSnapshots(folderId, token)
            runOnUiThread {
                selectedSnapshotId = snapshot.id
                persistSettings(
                    settings.copy(
                        gdrive = settings.gdrive.copy(
                            selectedSnapshotId = snapshot.id,
                            snapshots = snapshots,
                        ),
                    ),
                )
                refreshSnapshotListView()
                setSyncStatus("Uploaded snapshot ${snapshot.name}")
                updateSyncButtons()
            }
        }
    }

    private fun refreshDriveSnapshots() {
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        val folderId = settings.gdrive.folderId
        if (folderId.isBlank()) return setSyncStatus("Select a Drive folder first")

        runDriveOperation("Refreshing snapshots") {
            val snapshots = driveRepository.listSnapshots(folderId, token)
            runOnUiThread {
                selectedSnapshotId = DriveSnapshotSelection.preserveIfPresent(selectedSnapshotId, snapshots)
                persistSettings(
                    settings.copy(
                        gdrive = settings.gdrive.copy(
                            selectedSnapshotId = selectedSnapshotId,
                            snapshots = snapshots,
                        ),
                    ),
                )
                refreshSnapshotListView()
                setSyncStatus(if (snapshots.isEmpty()) "No snapshots found in Drive" else "Found ${snapshots.size} snapshot(s)")
                updateSyncButtons()
            }
        }
    }

    private fun confirmRestoreDriveSnapshot(snapshot: DriveSnapshotMeta) {
        AlertDialog.Builder(this)
            .setTitle("Restore snapshot")
            .setMessage("Replace local settings and notes with ${snapshot.name}?")
            .setPositiveButton("Restore") { _, _ -> restoreDriveSnapshot(snapshot) }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun restoreDriveSnapshot(snapshot: DriveSnapshotMeta) {
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        runDriveOperation("Restoring snapshot") {
            val settingsData = driveRepository.restoreSnapshot(snapshot.id, token, notesRepository)
            val restored = SettingsRepository.parse(settingsData.toString(Charsets.UTF_8))
            runOnUiThread {
                val preservedDrive = settings.gdrive.copy(selectedSnapshotId = snapshot.id)
                persistSettings(restored.copy(gdrive = preservedDrive))
                currentNotePath = settings.notesApp.currentNotePath
                selectedSnapshotId = snapshot.id
                refreshSnapshotListView()
                setSyncStatus("Restored snapshot ${snapshot.name}")
                showNotes()
            }
        }
    }

    private fun runDriveOperation(status: String, operation: () -> Unit) {
        setSyncStatus(status)
        setSyncButtonsEnabled(false)
        Thread {
            try {
                operation()
            } catch (error: Exception) {
                runOnUiThread {
                    if (error is DriveSnapshotRepository.DriveAuthorizationException) {
                        driveAccessToken = null
                    }
                    setSyncStatus(syncErrorStatus(error))
                    updateSyncButtons()
                }
            }
        }.start()
    }

    private fun refreshSnapshotListView() {
        syncFolderText?.text = selectedFolderLabel()
        syncSnapshotList?.adapter = SnapshotAdapter(settings.gdrive.snapshots)
        updateSyncButtons()
    }

    private fun setSyncStatus(status: String) {
        syncStatusText?.text = status
    }

    private fun updateSyncButtons() {
        val visibility = currentSyncActionVisibility()
        syncConnectButton?.visibility = if (visibility.connect) View.VISIBLE else View.GONE
        syncSelectFolderButton?.visibility = if (visibility.folderSelection) View.VISIBLE else View.GONE
        syncFolderText?.visibility = if (visibility.folderSelection) View.VISIBLE else View.GONE
        syncFolderTitle?.visibility = if (visibility.folderSelection) View.VISIBLE else View.GONE
        syncActionTitle?.visibility = if (visibility.upload || visibility.refresh) View.VISIBLE else View.GONE
        syncSnapshotTitle?.visibility = if (visibility.upload || visibility.refresh) View.VISIBLE else View.GONE
        syncSnapshotList?.visibility = if (visibility.upload || visibility.refresh) View.VISIBLE else View.GONE
        syncUploadButton?.visibility = if (visibility.upload) View.VISIBLE else View.GONE
        syncRefreshButton?.visibility = if (visibility.refresh) View.VISIBLE else View.GONE

        syncConnectButton?.isEnabled = visibility.connect
        syncSelectFolderButton?.isEnabled = visibility.folderSelection
        syncUploadButton?.isEnabled = visibility.upload
        syncRefreshButton?.isEnabled = visibility.refresh
    }

    private fun setSyncButtonsEnabled(enabled: Boolean) {
        syncConnectButton?.isEnabled = enabled
        syncSelectFolderButton?.isEnabled = enabled
        syncUploadButton?.isEnabled = enabled
        syncRefreshButton?.isEnabled = enabled
    }

    private fun currentSyncActionVisibility(): SyncActionVisibility {
        return SyncUiState.actionVisibility(
            connected = driveAccessToken != null,
            authInProgress = driveAuthInProgress,
            hasFolder = settings.gdrive.folderId.isNotBlank(),
            hasSelectedSnapshot = settings.gdrive.snapshots.any { it.id == selectedSnapshotId },
        )
    }

    private fun currentSyncStatus(): String {
        if (driveAccessToken == null) return "Not connected"
        val folderName = settings.gdrive.folderName.ifBlank { settings.gdrive.folderId }
        return if (folderName.isBlank()) "Connected, no folder selected" else "Synced to $folderName"
    }

    private fun selectedFolderLabel(): String {
        val folderName = settings.gdrive.folderName.ifBlank { settings.gdrive.folderId }
        return if (folderName.isBlank()) "No Drive folder selected" else "Drive folder: $folderName"
    }

    private fun showDriveFolderPicker() {
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        val rootFolder = DriveSnapshotRepository.DriveEntry(
            id = DriveSnapshotRepository.ROOT_FOLDER_ID,
            name = "My Drive",
            mimeType = DriveSnapshotRepository.DRIVE_FOLDER_MIME,
        )
        val folderStack = mutableListOf<DriveSnapshotRepository.DriveEntry>()
        var currentFolder = rootFolder
        var currentFolders: List<DriveSnapshotRepository.DriveEntry> = emptyList()

        val pickerLayout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(0, dp(8), 0, 0)
        }
        val locationText = TextView(this).apply {
            textSize = 15f
            setTextColor(COLOR_TEXT_PRIMARY)
            setPadding(dp(8), 0, dp(8), dp(8))
        }
        val folderList = ListView(this).apply {
            divider = null
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(260),
            )
        }
        val buttons = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.END
            setPadding(0, dp(10), 0, 0)
        }
        val newFolderButton = dialogButton("New folder")
        val backButton = dialogButton("Back")
        val selectButton = dialogButton("Select this folder")
        buttons.addView(newFolderButton)
        buttons.addView(backButton)
        buttons.addView(selectButton)
        pickerLayout.addView(locationText)
        pickerLayout.addView(folderList)
        pickerLayout.addView(buttons)

        val dialog = AlertDialog.Builder(this)
            .setTitle("Select Drive folder")
            .setView(pickerLayout)
            .setNegativeButton("Cancel", null)
            .create()

        fun showFolders() {
            locationText.text = "Loading ${currentFolder.name}"
            folderList.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_1, listOf("Loading..."))
            backButton.isEnabled = folderStack.isNotEmpty()
            Thread {
                try {
                    val folders = driveRepository.listFolders(currentFolder.id, token)
                    runOnUiThread {
                        currentFolders = folders
                        locationText.text = currentFolder.name
                        val labels = if (folders.isEmpty()) listOf("No folders") else folders.map { it.name }
                        folderList.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_1, labels)
                        folderList.setOnItemClickListener { _, _, position, _ ->
                            if (currentFolders.isEmpty()) return@setOnItemClickListener
                            folderStack.add(currentFolder)
                            currentFolder = currentFolders[position]
                            showFolders()
                        }
                        backButton.isEnabled = folderStack.isNotEmpty()
                    }
                } catch (error: Exception) {
                    runOnUiThread {
                        if (error is DriveSnapshotRepository.DriveAuthorizationException) {
                            driveAccessToken = null
                            dialog.dismiss()
                        }
                        setSyncStatus(syncErrorStatus(error))
                        updateSyncButtons()
                    }
                }
            }.start()
        }

        selectButton.setOnClickListener {
            saveSelectedDriveFolder(currentFolder)
            dialog.dismiss()
        }
        backButton.setOnClickListener {
            if (folderStack.isEmpty()) return@setOnClickListener
            currentFolder = folderStack.removeAt(folderStack.lastIndex)
            showFolders()
        }
        newFolderButton.setOnClickListener {
            promptNewDriveFolder(currentFolder, dialog)
        }

        dialog.setOnShowListener { showFolders() }
        dialog.show()
    }

    private fun promptNewDriveFolder(parentFolder: DriveSnapshotRepository.DriveEntry, pickerDialog: AlertDialog) {
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        val input = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_TEXT
            hint = "Folder name"
            setSingleLine(true)
        }
        AlertDialog.Builder(this)
            .setTitle("New Drive folder")
            .setView(input)
            .setPositiveButton("Create") { _, _ ->
                val folderName = input.text.toString().trim()
                if (folderName.isBlank()) {
                    setSyncStatus("Drive folder name is required")
                    return@setPositiveButton
                }
                setSyncStatus("Creating Drive folder")
                setSyncButtonsEnabled(false)
                Thread {
                    try {
                        val folder = driveRepository.createFolderIn(parentFolder.id, folderName, token)
                        runOnUiThread {
                            saveSelectedDriveFolder(folder)
                            pickerDialog.dismiss()
                        }
                    } catch (error: Exception) {
                        runOnUiThread {
                            if (error is DriveSnapshotRepository.DriveAuthorizationException) {
                                driveAccessToken = null
                                pickerDialog.dismiss()
                            }
                            setSyncStatus(syncErrorStatus(error))
                            updateSyncButtons()
                        }
                    }
                }.start()
            }
            .setNegativeButton("Cancel", null)
            .show()
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

    private inner class SnapshotAdapter(
        private val snapshots: List<DriveSnapshotMeta>,
    ) : BaseAdapter() {
        override fun getCount(): Int = snapshots.size
        override fun getItem(position: Int): DriveSnapshotMeta = snapshots[position]
        override fun getItemId(position: Int): Long = position.toLong()

        override fun getView(position: Int, convertView: View?, parent: ViewGroup?): View {
            val row = (convertView as? LinearLayout) ?: LinearLayout(this@MainActivity).apply {
                orientation = LinearLayout.HORIZONTAL
                gravity = Gravity.CENTER_VERTICAL
                setPadding(dp(12), dp(8), dp(8), dp(8))
            }
            row.removeAllViews()

            val snapshot = getItem(position)
            val label = TextView(this@MainActivity).apply {
                text = snapshotLabel(snapshot)
                textSize = 15f
                setTextColor(COLOR_TEXT_PRIMARY)
                layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
            }
            val restoreButton = ImageButton(this@MainActivity).apply {
                id = R.id.sync_snapshot_restore
                contentDescription = "Restore snapshot"
                setImageResource(R.drawable.ic_restore_24)
                setColorFilter(COLOR_ACCENT)
                scaleType = ImageView.ScaleType.CENTER
                setPadding(dp(9), dp(9), dp(9), dp(9))
                background = selectableBorderlessBackground()
                layoutParams = LinearLayout.LayoutParams(dp(44), dp(44)).apply {
                    leftMargin = dp(8)
                }
                setOnClickListener { confirmRestoreDriveSnapshot(snapshot) }
            }
            row.addView(label)
            row.addView(restoreButton)
            return row
        }
    }

    private fun snapshotLabel(snapshot: DriveSnapshotMeta): String {
        val createdAt = SnapshotDateFormatter.format(snapshot.createdAt)
        return if (createdAt.isBlank()) snapshot.name else "${snapshot.name}  $createdAt"
    }

    private fun syncErrorStatus(error: Exception): String {
        return when (error) {
            is DriveSnapshotRepository.SnapshotsFolderNotFoundException -> "No snapshots folder found in Drive"
            is DriveSnapshotRepository.DriveAuthorizationException -> "Google authorization expired. Connect Google again."
            is IOException -> "Drive API error: ${error.message ?: error.javaClass.simpleName}"
            else -> error.message ?: "Drive operation failed"
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
        window.statusBarColor = palette.statusBar
        window.navigationBarColor = palette.navigationBar
        window.decorView.systemUiVisibility = if (palette.lightSystemBars) {
            View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR or View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
        } else {
            0
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

    private enum class Screen {
        Notes,
        Pages,
        Sync,
        Settings,
    }

    companion object {
        private const val DRIVE_AUTH_REQUEST_CODE = 4201
        private const val PLAY_SERVICES_REQUEST_CODE = 4202
        private const val DRAWER_ANIMATION_MS = 180L
        private const val NOTE_AUTOSAVE_DELAY_MS = 600L
    }
}

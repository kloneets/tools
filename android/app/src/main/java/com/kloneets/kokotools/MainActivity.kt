package com.kloneets.kokotools

import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.content.IntentSender
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.text.Editable
import android.text.InputType
import android.text.TextWatcher
import android.view.Gravity
import android.view.ViewGroup
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.PopupMenu
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

    private lateinit var root: LinearLayout
    private lateinit var titleText: TextView
    private lateinit var subtitleText: TextView
    private lateinit var actionsButton: Button
    private lateinit var content: LinearLayout

    private var noteListView: ListView? = null
    private var noteEditor: EditText? = null

    private var firstInput: EditText? = null
    private var readInput: EditText? = null
    private var secondInput: EditText? = null
    private var resultText: TextView? = null

    private var syncStatusText: TextView? = null
    private var syncFolderInput: EditText? = null
    private var syncSnapshotList: ListView? = null
    private var syncConnectButton: Button? = null
    private var syncUploadButton: Button? = null
    private var syncRefreshButton: Button? = null
    private var syncRestoreButton: Button? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settingsRepository = SettingsRepository(this)
        notesRepository = NotesRepository(this)
        settings = settingsRepository.load()
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
        root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(COLOR_APP_BACKGROUND)
        }

        val toolbar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(18), dp(14), dp(14), dp(12))
            setBackgroundColor(COLOR_SURFACE)
        }

        val titleGroup = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
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

        actionsButton = toolbarButton("Actions", R.id.actions_menu) { showCurrentActionsMenu() }
        val toolsButton = toolbarButton("Tools", R.id.tools_menu) { showToolsMenu(it) }
        toolbar.addView(titleGroup)
        toolbar.addView(actionsButton)
        toolbar.addView(toolsButton)

        content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), dp(16), dp(16), dp(16))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                1f,
            )
        }

        root.addView(toolbar)
        root.addView(content)
        setContentView(root)
    }

    private fun toolbarButton(label: String, id: Int, action: (Button) -> Unit): Button {
        return Button(this).apply {
            text = label
            this.id = id
            minHeight = dp(40)
            minWidth = dp(80)
            setPadding(dp(12), 0, dp(12), 0)
            setTextColor(COLOR_ACCENT)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(8).toFloat())
            setOnClickListener { action(this) }
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT,
                dp(40),
            ).apply {
                leftMargin = dp(8)
            }
        }
    }

    private fun showToolsMenu(anchor: Button) {
        PopupMenu(this, anchor).apply {
            menu.add("Notes").setOnMenuItemClickListener {
                showNotes()
                true
            }
            menu.add("Pages").setOnMenuItemClickListener {
                showPages()
                true
            }
            menu.add("Sync").setOnMenuItemClickListener {
                showSync()
                true
            }
            show()
        }
    }

    private fun showCurrentActionsMenu() {
        when (titleText.text.toString()) {
            "Notes" -> showNotesActionsMenu(actionsButton)
            "Pages" -> showPagesActionsMenu(actionsButton)
            "Sync" -> showSyncActionsMenu(actionsButton)
        }
    }

    private fun showNotes() {
        setScreenHeader("Notes", "Markdown workspace")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL

        content.addView(sectionTitle("Local notes"))
        noteListView = ListView(this).apply {
            id = R.id.notes_list
            choiceMode = ListView.CHOICE_MODE_SINGLE
            divider = null
            background = roundedFill(COLOR_SURFACE, dp(10).toFloat())
            setPadding(0, dp(4), 0, dp(4))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                0.9f,
            )
            setOnItemClickListener { _, _, position, _ ->
                selectNote(noteList[position].relativePath)
            }
        }
        content.addView(noteListView)

        content.addView(sectionTitle("Editor"))
        noteEditor = EditText(this).apply {
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
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(10).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                2.2f,
            )
        }

        content.addView(noteEditor)
        refreshNotes()
    }

    private fun showNotesActionsMenu(anchor: Button) {
        PopupMenu(this, anchor).apply {
            menu.add("New note").setOnMenuItemClickListener {
                promptNewNote()
                true
            }
            menu.add("Save note").setOnMenuItemClickListener {
                saveCurrentNote()
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
        val adapter = ArrayAdapter(
            this,
            android.R.layout.simple_list_item_activated_1,
            noteList.map { it.relativePath },
        )
        noteListView?.adapter = adapter

        val selectedPath = currentNotePath.takeIf { path ->
            noteList.any { it.relativePath == path }
        } ?: noteList.firstOrNull()?.relativePath.orEmpty()

        if (selectedPath.isNotBlank()) {
            selectNote(selectedPath)
        } else {
            currentNotePath = ""
            noteEditor?.setText("")
        }
    }

    private fun selectNote(relativePath: String) {
        currentNotePath = relativePath
        noteEditor?.setText(notesRepository.read(relativePath))
        persistSettings(
            settings.copy(notesApp = settings.notesApp.copy(currentNotePath = relativePath)),
        )
        val index = noteList.indexOfFirst { it.relativePath == relativePath }
        if (index >= 0) {
            noteListView?.setItemChecked(index, true)
        }
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
                refreshNotes()
                selectNote(path)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun saveCurrentNote() {
        val path = currentNotePath.ifBlank { "untitled.md" }
        val saved = notesRepository.save(path, noteEditor?.text?.toString().orEmpty())
        currentNotePath = saved.relativePath
        persistSettings(
            settings.copy(notesApp = settings.notesApp.copy(currentNotePath = saved.relativePath)),
        )
        refreshNotes()
        Toast.makeText(this, "Saved ${saved.relativePath}", Toast.LENGTH_SHORT).show()
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
                persistSettings(settings.copy(notesApp = settings.notesApp.copy(currentNotePath = "")))
                refreshNotes()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun showPages() {
        setScreenHeader("Pages", "Reading progress converter")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL
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

    private fun showPagesActionsMenu(anchor: Button) {
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
        setScreenHeader("Sync", "Google Drive snapshots")
        content.removeAllViews()
        content.orientation = LinearLayout.VERTICAL

        syncStatusText = TextView(this).apply {
            id = R.id.sync_status
            text = if (driveAccessToken == null) "Not connected" else "Connected"
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

        content.addView(sectionTitle("Drive folder"))
        syncFolderInput = EditText(this).apply {
            id = R.id.sync_folder_id
            inputType = InputType.TYPE_CLASS_TEXT
            hint = "Google Drive folder ID"
            setText(settings.gdrive.folderId)
            setSingleLine(true)
            textSize = 15f
            setTextColor(COLOR_TEXT_PRIMARY)
            setHintTextColor(COLOR_TEXT_MUTED)
            setPadding(dp(14), 0, dp(14), 0)
            background = roundedStroke(COLOR_SURFACE, COLOR_BORDER, dp(10).toFloat())
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(52),
            )
        }
        content.addView(syncFolderInput)

        val saveFolderButton = commandButton("Set Drive folder ID", R.id.sync_save_folder) {
            saveDriveFolderId(refreshAfterSave = true)
        }
        content.addView(saveFolderButton)

        content.addView(sectionTitle("Snapshot actions"))
        syncConnectButton = commandButton("Connect Google", R.id.sync_connect) {
            connectGoogleDrive()
        }
        syncUploadButton = commandButton("Upload snapshot", R.id.sync_upload) {
            uploadDriveSnapshot()
        }
        syncRefreshButton = commandButton("Refresh snapshots", R.id.sync_refresh) {
            refreshDriveSnapshots()
        }
        syncRestoreButton = commandButton("Restore selected snapshot", R.id.sync_restore) {
            confirmRestoreDriveSnapshot()
        }
        content.addView(syncConnectButton)
        content.addView(syncUploadButton)
        content.addView(syncRefreshButton)
        content.addView(syncRestoreButton)

        content.addView(sectionTitle("Snapshots"))
        syncSnapshotList = ListView(this).apply {
            id = R.id.sync_snapshot_list
            choiceMode = ListView.CHOICE_MODE_SINGLE
            divider = null
            background = roundedFill(COLOR_SURFACE, dp(10).toFloat())
            setPadding(0, dp(4), 0, dp(4))
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                0,
                1f,
            )
            setOnItemClickListener { _, _, position, _ ->
                val snapshot = settings.gdrive.snapshots[position]
                selectedSnapshotId = snapshot.id
                persistSettings(
                    settings.copy(gdrive = settings.gdrive.copy(selectedSnapshotId = snapshot.id)),
                )
                updateSyncButtons()
            }
        }
        content.addView(syncSnapshotList)

        refreshSnapshotListView()
        updateSyncButtons()
    }

    private fun showSyncActionsMenu(anchor: Button) {
        PopupMenu(this, anchor).apply {
            menu.add("Connect Google").setOnMenuItemClickListener {
                connectGoogleDrive()
                true
            }
            menu.add("Upload snapshot").setOnMenuItemClickListener {
                uploadDriveSnapshot()
                true
            }
            menu.add("Refresh snapshots").setOnMenuItemClickListener {
                refreshDriveSnapshots()
                true
            }
            menu.add("Restore selected").setOnMenuItemClickListener {
                confirmRestoreDriveSnapshot()
                true
            }
            show()
        }
    }

    private fun persistSettings(next: AppSettings) {
        settings = next
        settingsRepository.save(settings)
    }

    private fun saveDriveFolderId(refreshAfterSave: Boolean = false) {
        val folderId = syncFolderInput?.text?.toString()?.trim() ?: settings.gdrive.folderId
        val folderChanged = folderId != settings.gdrive.folderId
        if (folderChanged) {
            selectedSnapshotId = ""
        }
        persistSettings(
            settings.copy(
                gdrive = settings.gdrive.copy(
                    folderId = folderId,
                    selectedSnapshotId = if (folderChanged) "" else settings.gdrive.selectedSnapshotId,
                    snapshots = if (folderChanged) emptyList() else settings.gdrive.snapshots,
                ),
            ),
        )
        if (folderChanged) {
            refreshSnapshotListView()
        }
        setSyncStatus(if (folderId.isBlank()) "Drive folder ID cleared" else "Drive folder ID saved")
        updateSyncButtons()
        if (refreshAfterSave && folderId.isNotBlank() && driveAccessToken != null) {
            refreshDriveSnapshots(saveFolderFirst = false)
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
            refreshDriveSnapshots(saveFolderFirst = false)
        }
    }

    private fun uploadDriveSnapshot() {
        saveDriveFolderId(refreshAfterSave = false)
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        val folderId = settings.gdrive.folderId
        if (folderId.isBlank()) return setSyncStatus("Set Drive folder ID first")

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

    private fun refreshDriveSnapshots(saveFolderFirst: Boolean = true) {
        if (saveFolderFirst) {
            saveDriveFolderId(refreshAfterSave = false)
        }
        val token = driveAccessToken ?: return setSyncStatus("Connect Google first")
        val folderId = settings.gdrive.folderId
        if (folderId.isBlank()) return setSyncStatus("Set Drive folder ID first")

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

    private fun confirmRestoreDriveSnapshot() {
        val snapshot = settings.gdrive.snapshots.firstOrNull { it.id == selectedSnapshotId }
            ?: return setSyncStatus("Select a snapshot first")
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
                    setSyncStatus(syncErrorStatus(error))
                    updateSyncButtons()
                }
            }
        }.start()
    }

    private fun refreshSnapshotListView() {
        val labels = settings.gdrive.snapshots.map {
            if (it.createdAt.isBlank()) it.name else "${it.name}  ${it.createdAt}"
        }
        syncSnapshotList?.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_activated_1, labels)
        val selectedIndex = settings.gdrive.snapshots.indexOfFirst { it.id == selectedSnapshotId }
        if (selectedIndex >= 0) {
            syncSnapshotList?.setItemChecked(selectedIndex, true)
        } else {
            syncSnapshotList?.clearChoices()
        }
        updateSyncButtons()
    }

    private fun setSyncStatus(status: String) {
        syncStatusText?.text = status
    }

    private fun updateSyncButtons() {
        val hasFolder = settings.gdrive.folderId.isNotBlank()
        val connected = driveAccessToken != null
        val hasSelectedSnapshot = settings.gdrive.snapshots.any { it.id == selectedSnapshotId }
        syncConnectButton?.isEnabled = !driveAuthInProgress
        syncUploadButton?.isEnabled = !driveAuthInProgress && connected && hasFolder
        syncRefreshButton?.isEnabled = !driveAuthInProgress && connected && hasFolder
        syncRestoreButton?.isEnabled = !driveAuthInProgress && connected && hasFolder && hasSelectedSnapshot
    }

    private fun setSyncButtonsEnabled(enabled: Boolean) {
        syncConnectButton?.isEnabled = enabled
        syncUploadButton?.isEnabled = enabled
        syncRefreshButton?.isEnabled = enabled
        syncRestoreButton?.isEnabled = enabled
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

    private fun dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }

    companion object {
        private const val DRIVE_AUTH_REQUEST_CODE = 4201
        private const val PLAY_SERVICES_REQUEST_CODE = 4202
        private val COLOR_APP_BACKGROUND = Color.rgb(245, 247, 250)
        private val COLOR_SURFACE = Color.rgb(255, 255, 255)
        private val COLOR_BORDER = Color.rgb(218, 225, 233)
        private val COLOR_STATUS = Color.rgb(232, 241, 255)
        private val COLOR_RESULT = Color.rgb(230, 247, 238)
        private val COLOR_ACCENT = Color.rgb(34, 102, 194)
        private val COLOR_TEXT_PRIMARY = Color.rgb(28, 35, 45)
        private val COLOR_TEXT_SECONDARY = Color.rgb(85, 97, 113)
        private val COLOR_TEXT_MUTED = Color.rgb(138, 149, 163)
    }
}

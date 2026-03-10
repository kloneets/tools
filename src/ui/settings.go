package ui

import (
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/kloneets/tools/src/gdrive"
	"github.com/kloneets/tools/src/helpers"
	"github.com/kloneets/tools/src/settings"
)

type Settings struct {
	SettingsButton       *gtk.Button
	panelBackdrop        *gtk.Box
	panelContainer       *gtk.Box
	showPages            *gtk.CheckButton
	showPassword         *gtk.CheckButton
	showNotes            *gtk.CheckButton
	resetWidgetsButton   *gtk.Button
	noteTabSpaces        *gtk.SpinButton
	noteVimMode          *gtk.CheckButton
	noteEditorMono       *gtk.CheckButton
	noteEditorFontSize   *gtk.SpinButton
	noteLineSpacing      *gtk.SpinButton
	noteFontSelect       *gtk.FontButton
	noteMonoFontSelect   *gtk.FontButton
	noteThemeSelect      *gtk.ComboBoxText
	enableDriveSync      *gtk.CheckButton
	driveSyncInterval    *gtk.SpinButton
	connectButton        *gtk.Button
	refreshFoldersButton *gtk.Button
	syncNowButton        *gtk.Button
	conflictLocalButton  *gtk.Button
	conflictRemoteButton *gtk.Button
	conflictLabel        *gtk.Label
	newFolderButton      *gtk.Button
	authLink             *gtk.LinkButton
	statusLabel          *gtk.Label
	lastSyncLabel        *gtk.Label
	trashList            *gtk.ListBox
	cleanTrashButton     *gtk.Button
	searchEntry          *gtk.Entry
	newFolderEntry       *gtk.Entry
	folderSelect         *gtk.ComboBoxText
	folders              map[string]gdrive.Folder
	allFolders           []gdrive.Folder
	authSession          *gdrive.AuthorizationSession
}

const folderGroupPrefix = "__group__:"

func (pm *PopoverMenu) NewSettings() *Settings {
	s := &Settings{
		folders: make(map[string]gdrive.Folder),
	}
	s.SettingsButton = gtk.NewButtonWithLabel("Settings")
	s.SettingsButton.ConnectClicked(func() {
		s.SettingsWindow(pm)
	})
	return s
}

func (s *Settings) SettingsWindow(pm *PopoverMenu) {
	if s.panelContainer != nil {
		pm.Popover.Hide()
		return
	}

	parentWindow := mainWindowFromPopover(pm)
	overlay := settingsOverlay()
	if overlay == nil {
		return
	}

	backdrop, panel := s.buildSettingsOverlay(parentWindow)
	s.panelBackdrop = backdrop
	s.panelContainer = panel
	overlay.AddOverlay(backdrop)
	overlay.SetMeasureOverlay(backdrop, false)
	overlay.AddOverlay(panel)
	overlay.SetMeasureOverlay(panel, false)

	settingsFrame := MainArea()
	settingsFrame.SetMarginTop(DefaultMasterPadding)
	settingsFrame.SetMarginStart(DefaultMasterPadding)
	settingsFrame.SetMarginEnd(DefaultMasterPadding)

	s.WidgetSettings(settingsFrame)
	s.NotesSettings(settingsFrame)
	s.GDriveSettings(parentWindow, settingsFrame)
	s.TrashSettings(settingsFrame)

	saveButton := gtk.NewButtonWithLabel("Save")
	saveButton.ConnectClicked(func() {
		if err := s.saveGDriveSettings(); err != nil {
			s.setStatus(err.Error())
			return
		}
		s.closeSettingsOverlay()
	})

	cancelButton := gtk.NewButtonWithLabel("Cancel")
	cancelButton.ConnectClicked(func() {
		s.closeSettingsOverlay()
	})

	buttonRow := gtk.NewBox(gtk.OrientationHorizontal, DefaultMasterPadding)
	buttonRow.SetMarginTop(DefaultMasterPadding)
	buttonRow.SetHAlign(gtk.AlignEnd)
	buttonRow.Append(cancelButton)
	buttonRow.Append(saveButton)

	content := MainArea()
	content.SetMarginTop(DefaultMasterPadding)
	content.SetMarginBottom(DefaultMasterPadding)
	content.SetMarginStart(DefaultMasterPadding)
	content.SetMarginEnd(DefaultMasterPadding)
	content.Append(settingsFrame)
	content.Append(buttonRow)

	scroll := gtk.NewScrolledWindow()
	scroll.SetHExpand(true)
	scroll.SetVExpand(true)
	scroll.SetChild(content)
	panel.Append(scroll)
	pm.Popover.Hide()
}

func (s *Settings) buildSettingsOverlay(parentWindow *gtk.Window) (*gtk.Box, *gtk.Box) {
	backdrop := gtk.NewBox(gtk.OrientationVertical, 0)
	backdrop.SetHExpand(true)
	backdrop.SetVExpand(true)
	backdrop.SetHAlign(gtk.AlignFill)
	backdrop.SetVAlign(gtk.AlignFill)
	backdrop.AddCSSClass("view")

	dismiss := gtk.NewGestureClick()
	dismiss.SetButton(1)
	dismiss.ConnectPressed(func(_ int, _ float64, _ float64) {
		s.closeSettingsOverlay()
	})
	gtk.BaseWidget(backdrop).AddController(dismiss)

	panel := gtk.NewBox(gtk.OrientationVertical, 0)
	panel.SetHAlign(gtk.AlignCenter)
	panel.SetVAlign(gtk.AlignCenter)
	panel.SetHExpand(true)
	panel.SetVExpand(true)
	panel.SetMarginTop(DefaultMasterPadding * 2)
	panel.SetMarginBottom(DefaultMasterPadding * 2)
	panel.SetMarginStart(DefaultMasterPadding * 2)
	panel.SetMarginEnd(DefaultMasterPadding * 2)
	panel.SetSizeRequest(920, 640)
	panel.AddCSSClass("card")

	header := gtk.NewHeaderBar()
	header.SetShowTitleButtons(false)
	header.SetTitleWidget(gtk.NewLabel("Settings"))
	closeButton := IconButton("window-close-symbolic", "Close settings")
	closeButton.ConnectClicked(func() {
		s.closeSettingsOverlay()
	})
	header.PackEnd(closeButton)
	panel.Append(header)

	return backdrop, panel
}

func (s *Settings) closeSettingsOverlay() {
	if s.authSession != nil {
		_ = s.authSession.Close()
		s.authSession = nil
	}

	overlay := settingsOverlay()
	if overlay != nil {
		if s.panelBackdrop != nil {
			overlay.RemoveOverlay(s.panelBackdrop)
		}
		if s.panelContainer != nil {
			overlay.RemoveOverlay(s.panelContainer)
		}
	}

	s.panelBackdrop = nil
	s.panelContainer = nil
}

func (s *Settings) GDriveSettings(window *gtk.Window, placeholder *gtk.Box) {
	current := settings.Inst().GDrive
	s.folders = make(map[string]gdrive.Folder)

	s.enableDriveSync = gtk.NewCheckButtonWithLabel("Enable Google Drive sync")
	s.enableDriveSync.SetActive(current.Enabled)
	s.driveSyncInterval = gtk.NewSpinButtonWithRange(1, 3600, 1)
	s.driveSyncInterval.SetValue(float64(current.SyncIntervalSec))

	s.connectButton = gtk.NewButtonWithLabel("Connect Google Drive")
	s.connectButton.ConnectClicked(func() {
		s.startDriveAuthorization(window)
	})

	s.authLink = gtk.NewLinkButtonWithLabel("", "Open Google authorization page")
	s.authLink.SetVisible(false)

	s.refreshFoldersButton = gtk.NewButtonWithLabel("Load Drive folders")
	s.refreshFoldersButton.ConnectClicked(func() {
		s.loadFolders(current.FolderID)
	})

	s.syncNowButton = gtk.NewButtonWithLabel("Sync now")
	s.syncNowButton.ConnectClicked(func() {
		s.syncNow()
	})
	s.conflictLocalButton = gtk.NewButtonWithLabel("Keep local")
	s.conflictLocalButton.ConnectClicked(func() {
		s.resolveConflictUseLocal()
	})
	s.conflictRemoteButton = gtk.NewButtonWithLabel("Use Drive")
	s.conflictRemoteButton.ConnectClicked(func() {
		s.resolveConflictUseRemote()
	})
	s.conflictLabel = InfoLabel("Remote data changed since the last sync. Choose whether to keep local changes or reload from Drive.")

	s.newFolderEntry = gtk.NewEntry()
	s.newFolderEntry.SetPlaceholderText("New Drive folder name")

	s.newFolderButton = gtk.NewButtonWithLabel("Create folder")
	s.newFolderButton.ConnectClicked(func() {
		s.createFolder()
	})

	s.searchEntry = gtk.NewEntry()
	s.searchEntry.SetPlaceholderText("Search folders")
	s.searchEntry.ConnectChanged(func() {
		s.populateFolderSelect(s.filterFolders(s.searchEntry.Text()), settings.Inst().GDrive.FolderID, settings.Inst().GDrive.FolderName)
	})

	s.folderSelect = gtk.NewComboBoxText()
	s.folderSelect.SetHExpand(true)
	s.folderSelect.ConnectChanged(func() {
		if id := s.folderSelect.ActiveID(); strings.HasPrefix(id, folderGroupPrefix) {
			s.folderSelect.SetActive(-1)
		}
	})
	s.populateFolderSelect(nil, current.FolderID, current.FolderName)

	s.statusLabel = InfoLabel("")
	s.lastSyncLabel = InfoLabel("")
	credentialsLabel := InfoLabel(fmt.Sprintf("Google OAuth client ID: %s", gdrive.OAuthClientLabel()))
	credentialsLabel.SetSelectable(true)

	s.updateStatusFromCurrentSettings(current)
	s.updateLastSyncLabel(current)

	connectionRow := FieldWrapper(SectionLabel("Connection"), DefaultBoxPadding)
	connectionRow.Append(s.connectButton)
	connectionRow.Append(s.syncNowButton)

	searchRow := FieldWrapper(SectionLabel("Find Drive folder"), DefaultBoxPadding)
	searchRow.Append(s.searchEntry)
	searchRow.Append(s.refreshFoldersButton)

	selectRow := FieldWrapper(SectionLabel("Drive sync folder"), DefaultBoxPadding)
	selectRow.Append(s.folderSelect)

	newFolderRow := FieldWrapper(SectionLabel("Create Drive folder"), DefaultBoxPadding)
	newFolderRow.Append(s.newFolderEntry)
	newFolderRow.Append(s.newFolderButton)

	content := MainArea()
	content.Append(s.enableDriveSync)
	intervalRow := FieldWrapper(SectionLabel("Sync interval (seconds)"), DefaultBoxPadding)
	intervalRow.Append(s.driveSyncInterval)
	content.Append(intervalRow)
	content.Append(credentialsLabel)
	content.Append(s.authLink)
	content.Append(connectionRow)
	content.Append(searchRow)
	content.Append(selectRow)
	content.Append(newFolderRow)
	conflictRow := FieldWrapper(SectionLabel("Conflict resolution"), DefaultBoxPadding)
	conflictRow.Append(s.conflictLabel)
	conflictRow.Append(s.conflictLocalButton)
	conflictRow.Append(s.conflictRemoteButton)
	content.Append(conflictRow)
	content.Append(s.statusLabel)
	content.Append(s.lastSyncLabel)

	gDriveFrame := Frame("Google Drive")
	gDriveFrame.SetChild(content)

	placeholder.Append(gDriveFrame)
	s.updateConflictControls(current)
}

func (s *Settings) NotesSettings(placeholder *gtk.Box) {
	current := settings.Inst().NotesApp

	s.noteTabSpaces = gtk.NewSpinButtonWithRange(1, 8, 1)
	s.noteTabSpaces.SetValue(float64(current.TabSpaces))
	s.noteVimMode = gtk.NewCheckButtonWithLabel("Enable Vim key bindings")
	s.noteVimMode.SetActive(current.VimMode)
	s.noteEditorMono = gtk.NewCheckButtonWithLabel("Use monospace font in editor")
	s.noteEditorMono.SetActive(current.EditorMonospace)
	s.noteEditorFontSize = gtk.NewSpinButtonWithRange(8, 40, 1)
	s.noteEditorFontSize.SetValue(float64(effectiveEditorFontSize(current)))
	s.noteLineSpacing = gtk.NewSpinButtonWithRange(0, 3, 0.05)
	s.noteLineSpacing.SetDigits(2)
	s.noteLineSpacing.SetValue(current.LineSpacing)
	s.noteFontSelect = fontButtonSelect(current.BodyFont, "Choose notes font", false)
	s.noteMonoFontSelect = fontButtonSelect(current.MonospaceFont, "Choose code font", true)
	s.noteThemeSelect = choiceSelect([][2]string{
		{"ide-dark", "IDE Dark"},
		{"neon-burst", "Neon Burst"},
		{"paper-light", "Paper Light"},
		{"mono", "Monochrome"},
	}, current.PreviewTheme)

	tabRow := FieldWrapper(SectionLabel("Tab spaces"), DefaultBoxPadding)
	tabRow.Append(s.noteTabSpaces)
	editorSizeRow := FieldWrapper(SectionLabel("Editor font size"), DefaultBoxPadding)
	editorSizeRow.Append(s.noteEditorFontSize)
	lineSpacingRow := FieldWrapper(SectionLabel("Line spacing"), DefaultBoxPadding)
	lineSpacingRow.Append(s.noteLineSpacing)
	fontRow := FieldWrapper(SectionLabel("Notes font"), DefaultBoxPadding)
	fontRow.Append(s.noteFontSelect)
	monoFontRow := FieldWrapper(SectionLabel("Code font"), DefaultBoxPadding)
	monoFontRow.Append(s.noteMonoFontSelect)
	themeRow := FieldWrapper(SectionLabel("Preview theme"), DefaultBoxPadding)
	themeRow.Append(s.noteThemeSelect)

	content := MainArea()
	content.Append(tabRow)
	content.Append(editorSizeRow)
	content.Append(lineSpacingRow)
	content.Append(s.noteVimMode)
	content.Append(s.noteEditorMono)
	content.Append(fontRow)
	content.Append(monoFontRow)
	content.Append(themeRow)

	frame := Frame("Notes")
	frame.SetChild(content)
	placeholder.Append(frame)
}

func (s *Settings) TrashSettings(placeholder *gtk.Box) {
	s.trashList = gtk.NewListBox()
	s.trashList.SetSelectionMode(gtk.SelectionNone)
	s.cleanTrashButton = gtk.NewButtonWithLabel("Clean trash")
	s.cleanTrashButton.ConnectClicked(func() {
		if err := settings.CleanTrash(); err != nil {
			s.setStatus("Trash clean failed: " + err.Error())
			return
		}
		s.reloadTrashItems()
		s.setStatus("Trash cleaned.")
	})

	header := FieldWrapper(SectionLabel("Trash"), DefaultBoxPadding)
	header.Append(InfoLabel("Right-click a trash item to restore it."))
	header.Append(s.cleanTrashButton)

	content := MainArea()
	content.Append(header)
	content.Append(s.trashList)

	frame := Frame("Notes trash")
	frame.SetChild(content)
	placeholder.Append(frame)

	s.reloadTrashItems()
}

func (s *Settings) reloadTrashItems() {
	if s.trashList == nil {
		return
	}

	for child := s.trashList.FirstChild(); child != nil; {
		next := gtk.BaseWidget(child).NextSibling()
		s.trashList.Remove(child)
		child = next
	}

	items, err := settings.ListTrashItems()
	if err != nil {
		row := gtk.NewListBoxRow()
		row.SetChild(InfoLabel("Could not load trash: " + err.Error()))
		s.trashList.Append(row)
		return
	}
	if len(items) == 0 {
		row := gtk.NewListBoxRow()
		row.SetChild(InfoLabel("Trash is empty."))
		s.trashList.Append(row)
		return
	}

	for _, item := range items {
		s.trashList.Append(s.trashListRow(item))
	}
}

func (s *Settings) trashListRow(item settings.TrashItem) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()

	pathLabel := gtk.NewLabel(filepath.ToSlash(item.RelPath))
	pathLabel.SetXAlign(0)
	pathLabel.SetSelectable(true)

	kind := "Note"
	if item.IsDir {
		kind = "Folder"
	}
	kindLabel := InfoLabel(kind)

	box := gtk.NewBox(gtk.OrientationHorizontal, DefaultBoxPadding)
	box.SetMarginTop(DefaultBoxPadding)
	box.SetMarginBottom(DefaultBoxPadding)
	box.SetMarginStart(DefaultBoxPadding)
	box.SetMarginEnd(DefaultBoxPadding)
	box.Append(pathLabel)
	box.Append(kindLabel)
	row.SetChild(box)

	popover := s.trashRestorePopover(row, item)
	click := gtk.NewGestureClick()
	click.SetButton(3)
	click.ConnectPressed(func(_ int, x, y float64) {
		rect := &gdk.Rectangle{}
		rect.SetX(int(x))
		rect.SetY(int(y))
		rect.SetWidth(1)
		rect.SetHeight(1)
		popover.SetPointingTo(rect)
		if canPopupSettingsPopover(row) {
			popover.Popup()
		}
	})
	gtk.BaseWidget(row).AddController(click)

	return row
}

func (s *Settings) trashRestorePopover(parent gtk.Widgetter, item settings.TrashItem) *gtk.Popover {
	popover := gtk.NewPopover()
	popover.SetHasArrow(true)
	popover.SetAutohide(true)
	popover.SetParent(parent)

	restoreButton := gtk.NewButtonWithLabel("Restore")
	restoreButton.ConnectClicked(func() {
		target, err := settings.RestoreTrashItem(item.Path)
		if err != nil {
			s.setStatus("Restore failed: " + err.Error())
			return
		}
		popover.Popdown()
		s.reloadTrashItems()
		relPath, relErr := filepath.Rel(settings.NotesRootDir(), target)
		if relErr != nil {
			s.setStatus("Restored from trash: " + filepath.Base(target))
			return
		}
		s.setStatus("Restored from trash: " + filepath.ToSlash(relPath))
	})

	box := gtk.NewBox(gtk.OrientationVertical, DefaultBoxPadding)
	box.SetMarginTop(DefaultBoxPadding)
	box.SetMarginBottom(DefaultBoxPadding)
	box.SetMarginStart(DefaultBoxPadding)
	box.SetMarginEnd(DefaultBoxPadding)
	box.Append(InfoLabel(filepath.ToSlash(item.RelPath)))
	box.Append(restoreButton)
	popover.SetChild(box)
	return popover
}

func (s *Settings) WidgetSettings(placeholder *gtk.Box) {
	current := settings.Inst().UI

	s.showPages = gtk.NewCheckButtonWithLabel("Show Pages")
	s.showPages.SetActive(current.ShowPages)
	s.showPassword = gtk.NewCheckButtonWithLabel("Show Password generator")
	s.showPassword.SetActive(current.ShowPassword)
	s.showNotes = gtk.NewCheckButtonWithLabel("Show Notes")
	s.showNotes.SetActive(current.ShowNotes)
	s.resetWidgetsButton = gtk.NewButtonWithLabel("Reset visible widgets")
	s.resetWidgetsButton.ConnectClicked(func() {
		s.showPages.SetActive(true)
		s.showPassword.SetActive(true)
		s.showNotes.SetActive(true)
	})

	content := MainArea()
	content.Append(s.showPages)
	content.Append(s.showPassword)
	content.Append(s.showNotes)
	content.Append(s.resetWidgetsButton)

	frame := Frame("Visible widgets")
	frame.SetChild(content)
	placeholder.Append(frame)
}

func (s *Settings) startDriveAuthorization(window *gtk.Window) {
	if s.authSession != nil {
		s.setStatus("Authorization is already in progress.")
		return
	}

	url, session, err := gdrive.StartLocalAuthorization()
	if err != nil {
		s.setStatus("Google Drive setup error: " + err.Error())
		return
	}

	s.authSession = session
	s.authLink.SetURI(url)
	s.authLink.SetVisible(true)
	s.setStatus("Browser opened. Finish Google authorization and return to the app.")
	helpers.OpenURI(url)

	go func() {
		err := <-session.Wait()
		coreglib.IdleAdd(func() {
			s.authSession = nil
			if err != nil {
				s.setStatus("Authorization failed: " + err.Error())
				return
			}
			s.setStatus("Google Drive connected. Load folders and choose the sync target.")
			s.loadFolders("")
		})
	}()

	window.Present()
}

func (s *Settings) loadFolders(selectedID string) {
	folders, err := gdrive.ListFolders()
	if err != nil {
		s.setStatus("Could not load Drive folders: " + err.Error())
		return
	}

	s.allFolders = folders
	s.populateFolderSelect(s.filterFolders(s.searchEntry.Text()), selectedID, settings.Inst().GDrive.FolderName)
	if len(folders) == 0 {
		s.setStatus("No Google Drive folders found for this account.")
		return
	}

	s.setStatus(fmt.Sprintf("Loaded %d Google Drive folders.", len(folders)))
}

func (s *Settings) populateFolderSelect(folders []gdrive.Folder, selectedID string, selectedName string) {
	s.folderSelect.RemoveAll()
	s.folders = make(map[string]gdrive.Folder)

	if selectedID != "" && selectedName != "" {
		s.folderSelect.Append(selectedID, selectedName)
		s.folders[selectedID] = gdrive.Folder{ID: selectedID, Name: selectedName, Path: selectedName}
	}

	lastGroup := ""
	labelCounts := countFolderLabels(folders)
	for _, folder := range folders {
		if _, exists := s.folders[folder.ID]; exists {
			continue
		}
		if folder.TopLevel != "" && folder.TopLevel != lastGroup {
			groupID := folderGroupPrefix + folder.TopLevel
			s.folderSelect.Append(groupID, "── "+folder.TopLevel+" ──")
			lastGroup = folder.TopLevel
		}
		s.folderSelect.Append(folder.ID, folderDisplayLabel(folder, labelCounts))
		s.folders[folder.ID] = folder
	}

	if selectedID != "" {
		s.folderSelect.SetActiveID(selectedID)
	}
}

func (s *Settings) updateStatusFromCurrentSettings(current *settings.GDriveSettings) {
	switch {
	case current == nil:
		s.setStatus("Google Drive sync is not configured yet.")
	case current.LastSyncStatus == "conflict" && current.LastSyncMessage != "":
		s.setStatus("Drive sync conflict: " + current.LastSyncMessage)
	case current.LastSyncStatus == "error" && current.LastSyncMessage != "":
		s.setStatus("Last Drive sync failed: " + current.LastSyncMessage)
	case current.Ready():
		s.setStatus("Google Drive sync is enabled for folder: " + current.FolderName)
	case gdrive.HasCredentials():
		s.setStatus("Google OAuth client ID is configured. Connect Google Drive and choose a folder to enable sync.")
	default:
		s.setStatus("Missing Google OAuth client ID. Set KOKO_TOOLS_GOOGLE_CLIENT_ID or compile it into the app.")
	}
}

func (s *Settings) updateLastSyncLabel(current *settings.GDriveSettings) {
	s.lastSyncLabel.SetText(lastSyncSummary(current))
}

func (s *Settings) updateConflictControls(current *settings.GDriveSettings) {
	visible := current != nil && current.LastSyncStatus == "conflict"
	if s.conflictLabel != nil {
		s.conflictLabel.SetVisible(visible)
	}
	if s.conflictLocalButton != nil {
		s.conflictLocalButton.SetVisible(visible)
	}
	if s.conflictRemoteButton != nil {
		s.conflictRemoteButton.SetVisible(visible)
	}
}

func (s *Settings) applyFormToSettings() {
	if s.showPages != nil {
		settings.Inst().UI.ShowPages = s.showPages.Active()
	}
	if s.showPassword != nil {
		settings.Inst().UI.ShowPassword = s.showPassword.Active()
	}
	if s.showNotes != nil {
		settings.Inst().UI.ShowNotes = s.showNotes.Active()
	}
	if s.noteTabSpaces != nil {
		settings.Inst().NotesApp.TabSpaces = s.noteTabSpaces.ValueAsInt()
	}
	if s.noteVimMode != nil {
		settings.Inst().NotesApp.VimMode = s.noteVimMode.Active()
	}
	if s.noteEditorMono != nil {
		settings.Inst().NotesApp.EditorMonospace = s.noteEditorMono.Active()
	}
	if s.noteEditorFontSize != nil {
		settings.Inst().NotesApp.EditorFontSize = s.noteEditorFontSize.ValueAsInt()
	}
	if s.noteLineSpacing != nil {
		settings.Inst().NotesApp.LineSpacing = s.noteLineSpacing.Value()
	}
	if s.noteFontSelect != nil && s.noteFontSelect.Font() != "" {
		settings.Inst().NotesApp.BodyFont = s.noteFontSelect.Font()
	}
	if s.noteMonoFontSelect != nil && s.noteMonoFontSelect.Font() != "" {
		settings.Inst().NotesApp.MonospaceFont = s.noteMonoFontSelect.Font()
	}
	if s.noteThemeSelect != nil && s.noteThemeSelect.ActiveID() != "" {
		settings.Inst().NotesApp.PreviewTheme = s.noteThemeSelect.ActiveID()
	}

	g := settings.Inst().GDrive
	g.Enabled = s.enableDriveSync.Active()
	if s.driveSyncInterval != nil {
		g.SyncIntervalSec = s.driveSyncInterval.ValueAsInt()
	}

	selectedID := s.folderSelect.ActiveID()
	if selectedID != "" && !strings.HasPrefix(selectedID, folderGroupPrefix) {
		g.FolderID = selectedID
		if folder, ok := s.folders[selectedID]; ok {
			if folder.Path != "" {
				g.FolderName = folder.Path
			} else {
				g.FolderName = folder.Name
			}
		} else {
			g.FolderName = s.folderSelect.ActiveText()
		}
	}
}

func (s *Settings) validateDriveSettings() error {
	g := settings.Inst().GDrive

	if !g.Enabled {
		return nil
	}
	if !gdrive.HasCredentials() {
		return fmt.Errorf("missing Google OAuth client ID")
	}
	if !gdrive.HasToken() {
		return fmt.Errorf("connect Google Drive before enabling sync")
	}
	if g.FolderID == "" {
		return fmt.Errorf("choose a Google Drive folder for sync")
	}

	return nil
}

func (s *Settings) saveGDriveSettings() error {
	s.applyFormToSettings()
	settings.SaveSettings()
	if settings.Inst().GDrive.Enabled && settings.Inst().GDrive.FolderID == "" {
		s.setStatus("Google Drive sync is enabled. Connect Drive and choose a folder to start auto-syncing.")
	}
	s.updateLastSyncLabel(settings.Inst().GDrive)
	s.updateConflictControls(settings.Inst().GDrive)
	return nil
}

func (s *Settings) syncNow() {
	s.applyFormToSettings()
	if err := s.validateDriveSettings(); err != nil {
		s.setStatus(err.Error())
		return
	}
	if err := settings.SyncDriveData(); err != nil {
		s.setStatus("Drive sync failed: " + err.Error())
		s.updateLastSyncLabel(settings.Inst().GDrive)
		s.updateConflictControls(settings.Inst().GDrive)
		return
	}
	s.setStatus(settings.Inst().GDrive.LastSyncMessage)
	s.updateLastSyncLabel(settings.Inst().GDrive)
	s.updateConflictControls(settings.Inst().GDrive)
}

func (s *Settings) resolveConflictUseLocal() {
	if err := settings.ResolveDriveConflictUseLocal(); err != nil {
		s.setStatus("Conflict resolution failed: " + err.Error())
		return
	}
	s.setStatus(settings.Inst().GDrive.LastSyncMessage)
	s.updateLastSyncLabel(settings.Inst().GDrive)
	s.updateConflictControls(settings.Inst().GDrive)
}

func (s *Settings) resolveConflictUseRemote() {
	backupDir, err := settings.ResolveDriveConflictUseRemote()
	if err != nil {
		s.setStatus("Conflict resolution failed: " + err.Error())
		return
	}
	s.setStatus("Drive version restored. Local data was backed up to: " + backupDir)
	s.updateLastSyncLabel(settings.Inst().GDrive)
	s.updateConflictControls(settings.Inst().GDrive)
}

func (s *Settings) setStatus(message string) {
	log.Println(message)
	s.statusLabel.SetText(message)
}

func (s *Settings) createFolder() {
	if !gdrive.HasCredentials() {
		s.setStatus("missing Google OAuth client ID")
		return
	}
	if !gdrive.HasToken() {
		s.setStatus("connect Google Drive before creating a folder")
		return
	}

	name := strings.TrimSpace(s.newFolderEntry.Text())
	if name == "" {
		s.setStatus("enter a folder name first")
		return
	}

	parentID := s.selectedFolderID()
	folder, err := gdrive.CreateFolder(name, parentID)
	if err != nil {
		s.setStatus("could not create Drive folder: " + err.Error())
		return
	}

	s.newFolderEntry.SetText("")
	s.loadFolders(folder.ID)
	s.setStatus("Created Drive folder: " + folder.Name)
}

func (s *Settings) selectedFolderID() string {
	selectedID := s.folderSelect.ActiveID()
	if strings.HasPrefix(selectedID, folderGroupPrefix) {
		return ""
	}
	return selectedID
}

func (s *Settings) filterFolders(query string) []gdrive.Folder {
	if query == "" {
		return s.allFolders
	}

	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]gdrive.Folder, 0, len(s.allFolders))
	for _, folder := range s.allFolders {
		haystack := strings.ToLower(folder.Path + " " + folder.Name + " " + folder.TopLevel + " " + folder.ID)
		if strings.Contains(haystack, query) {
			filtered = append(filtered, folder)
		}
	}

	return filtered
}

func countFolderLabels(folders []gdrive.Folder) map[string]int {
	counts := make(map[string]int, len(folders))
	for _, folder := range folders {
		counts[folder.Path]++
	}
	return counts
}

func folderDisplayLabel(folder gdrive.Folder, labelCounts map[string]int) string {
	label := folder.Path
	if labelCounts[folder.Path] > 1 {
		label += " (" + shortFolderID(folder.ID) + ")"
	}
	return label
}

func shortFolderID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func fontButtonSelect(current string, title string, monospaceOnly bool) *gtk.FontButton {
	if strings.TrimSpace(current) == "" {
		current = "Cantarell 11"
		if monospaceOnly {
			current = "Noto Sans Mono 11"
		}
	}
	button := gtk.NewFontButtonWithFont(current)
	button.SetTitle(title)
	button.SetModal(true)
	button.SetUseFont(false)
	button.SetUseSize(true)
	button.SetLevel(gtk.FontChooserLevelFamily | gtk.FontChooserLevelSize)
	button.SetPreviewText("AaBbYyZz 0123456789")
	if monospaceOnly {
		button.SetFilterFunc(func(family pango.FontFamilier, _ pango.FontFacer) bool {
			return pango.BaseFontFamily(family).IsMonospace()
		})
	}
	return button
}

func effectiveEditorFontSize(current settings.NotesAppSettings) int {
	if current.EditorFontSize > 0 {
		return current.EditorFontSize
	}
	if current.EditorMonospace {
		return fontSpecSize(current.MonospaceFont, 11)
	}
	return fontSpecSize(current.BodyFont, 11)
}

func fontSpecSize(spec string, fallback int) int {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return fallback
	}
	parts := strings.Fields(spec)
	if len(parts) == 0 {
		return fallback
	}
	size, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || size <= 0 {
		return fallback
	}
	return size
}

func choiceSelect(options [][2]string, current string) *gtk.ComboBoxText {
	selectBox := gtk.NewComboBoxText()
	selectBox.SetHExpand(true)
	for _, option := range options {
		selectBox.Append(option[0], option[1])
	}
	if current != "" {
		selectBox.SetActiveID(current)
	}
	return selectBox
}

func canPopupSettingsPopover(parent gtk.Widgetter) bool {
	if parent == nil {
		return false
	}
	widget := gtk.BaseWidget(parent)
	return widget.Root() != nil && widget.Native() != nil && widget.Mapped()
}

func settingsOverlay() *gtk.Overlay {
	if globals := helpers.CurrentGlobals(); globals != nil {
		return globals.MainOverlay
	}
	return nil
}

func mainWindowFromPopover(pm *PopoverMenu) *gtk.Window {
	if globals := helpers.CurrentGlobals(); globals != nil && globals.MainWindow != nil {
		return globals.MainWindow
	}
	if pm == nil || pm.Popover == nil {
		return nil
	}

	root := gtk.BaseWidget(pm.Popover).Root()
	if root == nil {
		return nil
	}

	window, _ := root.Cast().(*gtk.Window)
	return window
}

func lastSyncSummary(current *settings.GDriveSettings) string {
	if current == nil || current.LastSyncAt == "" {
		return "Last sync: not run yet"
	}
	if (current.LastSyncStatus == "error" || current.LastSyncStatus == "conflict") && current.LastSyncMessage != "" {
		return fmt.Sprintf("Last sync: %s (%s)\n%s", current.LastSyncAt, current.LastSyncStatus, current.LastSyncMessage)
	}

	return fmt.Sprintf("Last sync: %s (%s)", current.LastSyncAt, current.LastSyncStatus)
}

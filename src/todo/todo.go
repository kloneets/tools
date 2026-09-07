package todo

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kloneets/tools/src/helpers"
)

const (
	SchemaVersion    = 1
	DefaultListID    = "default"
	DefaultListName  = "Default"
	defaultListRev   = 1
	ListsFile        = "todo_lists.json"
	NamedListsDir    = "todo-lists"
	StatusTodo       = "todo"
	StatusDone       = "done"
	StatusArchived   = "archived"
	TermShort        = "short"
	TermLong         = "long"
	CheckedDelay     = 10 * time.Second
	DoneArchiveAfter = 7 * 24 * time.Hour
)

type Store struct {
	Version       int      `json:"version"`
	Items         []Item   `json:"items"`
	ArchiveMonths []string `json:"archive_months,omitempty"`
}

type ListMeta struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Rev       int64      `json:"rev"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Deleted   bool       `json:"deleted,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type ListsStore struct {
	Version       int        `json:"version"`
	CurrentListID string     `json:"current_list_id"`
	Lists         []ListMeta `json:"lists"`
}

type Item struct {
	ID         string     `json:"id"`
	Text       string     `json:"text"`
	Status     string     `json:"status"`
	Term       string     `json:"term"`
	Order      int        `json:"order"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	CheckedAt  *time.Time `json:"checked_at"`
	DoneAt     *time.Time `json:"done_at"`
	ArchivedAt *time.Time `json:"archived_at"`
}

type Repository struct {
	path          string
	baseDir       string
	currentListID string
	now           func() time.Time
	newID         func() (string, error)
}

func NewRepository() *Repository {
	return NewRepositoryAt(DefaultPath())
}

func NewRepositoryAt(path string) *Repository {
	return &Repository{
		path:          path,
		baseDir:       filepath.Dir(path),
		currentListID: DefaultListID,
		now:           time.Now,
		newID:         randomListID,
	}
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "todos.json"
	}
	return filepath.Join(home, helpers.AppConfigMainDir, helpers.AppConfigAppDir, "todos.json")
}

func (r *Repository) SetNowForTests(now func() time.Time) {
	r.now = now
}

func (r *Repository) SetListIDGeneratorForTests(newID func() (string, error)) {
	if r == nil {
		return
	}
	r.newID = newID
}

func (r *Repository) Path() string {
	if r == nil {
		return ""
	}
	return r.storePath(r.currentListID)
}

func (r *Repository) CurrentListID() string {
	if r == nil {
		return DefaultListID
	}
	id := NormalizeListID(r.currentListID)
	if id == "" {
		return DefaultListID
	}
	return id
}

func (r *Repository) ListsPath() string {
	if r == nil {
		return ""
	}
	return filepath.Join(r.baseDir, ListsFile)
}

func (r *Repository) LoadLists() (ListsStore, error) {
	catalog, err := readListsStore(r.ListsPath(), r.currentTime())
	if err != nil {
		return ListsStore{}, err
	}
	current := r.CurrentListID()
	if !hasActiveList(catalog, current) {
		current = DefaultListID
	}
	r.currentListID = current
	catalog.CurrentListID = current
	return catalog, nil
}

func (r *Repository) SaveLists(catalog ListsStore) error {
	NormalizeLists(&catalog, r.currentTime())
	current := r.CurrentListID()
	if !hasActiveList(catalog, current) {
		current = DefaultListID
	}
	catalog.CurrentListID = current
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todo lists: %w", err)
	}
	if err := os.MkdirAll(r.baseDir, 0o755); err != nil {
		return fmt.Errorf("create todo directory: %w", err)
	}
	if err := os.WriteFile(r.ListsPath(), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write todo lists: %w", err)
	}
	r.currentListID = current
	return nil
}

func (r *Repository) SelectList(id string) (ListsStore, error) {
	id = NormalizeListID(id)
	catalog, err := r.LoadLists()
	if err != nil {
		return ListsStore{}, err
	}
	if !hasActiveList(catalog, id) {
		return catalog, fmt.Errorf("todo list %q does not exist", id)
	}
	catalog.CurrentListID = id
	r.currentListID = id
	return catalog, nil
}

func (r *Repository) CycleList(delta int) (ListsStore, error) {
	catalog, err := r.LoadLists()
	if err != nil {
		return ListsStore{}, err
	}
	active := ActiveLists(catalog)
	if len(active) == 0 || delta == 0 {
		return catalog, nil
	}
	index := 0
	for i, list := range active {
		if list.ID == catalog.CurrentListID {
			index = i
			break
		}
	}
	next := (index + delta) % len(active)
	if next < 0 {
		next += len(active)
	}
	catalog.CurrentListID = active[next].ID
	r.currentListID = catalog.CurrentListID
	return catalog, nil
}

func (r *Repository) CreateList(name string) (ListsStore, ListMeta, error) {
	name = NormalizeListName(name)
	if name == "" {
		return ListsStore{}, ListMeta{}, errors.New("todo list name is required")
	}
	catalog, err := r.LoadLists()
	if err != nil {
		return ListsStore{}, ListMeta{}, err
	}
	if listNameTaken(catalog, name, "") {
		return catalog, ListMeta{}, fmt.Errorf("todo list %q already exists", name)
	}
	id, err := r.generateListID(catalog)
	if err != nil {
		return catalog, ListMeta{}, err
	}
	now := r.currentTime()
	list := ListMeta{ID: id, Name: name, Rev: revForTime(now), CreatedAt: now, UpdatedAt: now}
	catalog.Lists = append(catalog.Lists, list)
	catalog.CurrentListID = id
	r.currentListID = id
	if err := r.SaveLists(catalog); err != nil {
		return ListsStore{}, ListMeta{}, err
	}
	if err := r.SaveList(id, Store{}); err != nil {
		return ListsStore{}, ListMeta{}, err
	}
	catalog, _ = r.LoadLists()
	if saved, ok := ListByID(catalog, id); ok {
		list = saved
	}
	return catalog, list, nil
}

func (r *Repository) RenameList(id string, name string) (ListsStore, error) {
	id = NormalizeListID(id)
	name = NormalizeListName(name)
	if name == "" {
		return ListsStore{}, errors.New("todo list name is required")
	}
	catalog, err := r.LoadLists()
	if err != nil {
		return ListsStore{}, err
	}
	if listNameTaken(catalog, name, id) {
		return catalog, fmt.Errorf("todo list %q already exists", name)
	}
	now := r.currentTime()
	found := false
	for i := range catalog.Lists {
		if catalog.Lists[i].ID != id || catalog.Lists[i].Deleted {
			continue
		}
		catalog.Lists[i].Name = name
		catalog.Lists[i].Rev = revForTime(now)
		catalog.Lists[i].UpdatedAt = now
		found = true
		break
	}
	if !found {
		return catalog, fmt.Errorf("todo list %q does not exist", id)
	}
	if err := r.SaveLists(catalog); err != nil {
		return ListsStore{}, err
	}
	return r.LoadLists()
}

func (r *Repository) DeleteCurrentListIfEmpty() (ListsStore, error) {
	return r.DeleteCurrentList()
}

func (r *Repository) DeleteCurrentList() (ListsStore, error) {
	return r.DeleteList(r.CurrentListID())
}

func (r *Repository) DeleteList(id string) (ListsStore, error) {
	id = NormalizeListID(id)
	if id == DefaultListID {
		catalog, err := r.LoadLists()
		if err != nil {
			return ListsStore{}, err
		}
		return catalog, errors.New("Default todo list cannot be deleted")
	}
	catalog, err := r.LoadLists()
	if err != nil {
		return ListsStore{}, err
	}
	now := r.currentTime()
	found := false
	for i := range catalog.Lists {
		if catalog.Lists[i].ID != id {
			continue
		}
		catalog.Lists[i].Deleted = true
		catalog.Lists[i].DeletedAt = &now
		catalog.Lists[i].UpdatedAt = now
		catalog.Lists[i].Rev = revForTime(now)
		found = true
		break
	}
	if !found {
		return catalog, fmt.Errorf("todo list %q does not exist", id)
	}
	if r.CurrentListID() == id {
		catalog.CurrentListID = DefaultListID
		r.currentListID = DefaultListID
	}
	if err := r.SaveLists(catalog); err != nil {
		return ListsStore{}, err
	}
	if err := os.Remove(r.storePath(id)); err != nil && !os.IsNotExist(err) {
		return catalog, fmt.Errorf("delete todo list data: %w", err)
	}
	catalog, _ = r.LoadLists()
	return catalog, nil
}

func (r *Repository) RemoveDeletedListData(catalog ListsStore) error {
	if r == nil {
		return nil
	}
	for _, list := range catalog.Lists {
		if !list.Deleted || list.ID == DefaultListID {
			continue
		}
		if err := os.Remove(r.storePath(list.ID)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete todo list data: %w", err)
		}
	}
	return nil
}

func (r *Repository) CurrentList() (ListMeta, error) {
	catalog, err := r.LoadLists()
	if err != nil {
		return ListMeta{}, err
	}
	for _, list := range catalog.Lists {
		if list.ID == catalog.CurrentListID && !list.Deleted {
			return list, nil
		}
	}
	return defaultListMeta(), nil
}

func (r *Repository) LoadList(id string) (Store, error) {
	store, err := readStore(r.storePath(id))
	if err != nil {
		return Store{}, err
	}
	changed := Cleanup(&store, r.currentTime())
	if changed {
		if err := r.SaveList(id, store); err != nil {
			return Store{}, err
		}
	}
	return store, nil
}

func (r *Repository) SaveList(id string, store Store) error {
	store.Version = SchemaVersion
	Normalize(&store)
	Cleanup(&store, r.currentTime())
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todos: %w", err)
	}
	path := r.storePath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create todo directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write todos: %w", err)
	}
	return nil
}

func (r *Repository) Load() (Store, error) {
	return r.LoadList(r.currentListID)
}

func (r *Repository) Save(store Store) error {
	return r.SaveList(r.currentListID, store)
}

func (r *Repository) Add(text string) (Store, Item, error) {
	store, err := r.Load()
	if err != nil {
		return Store{}, Item{}, err
	}
	now := r.currentTime()
	item := Item{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		Text:      strings.TrimSpace(text),
		Status:    StatusTodo,
		Term:      TermShort,
		Order:     nextActiveOrder(store.Items),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if item.Text == "" {
		return Store{}, Item{}, errors.New("todo text is required")
	}
	store.Items = append(store.Items, item)
	return store, item, r.Save(store)
}

func (r *Repository) Toggle(id string) (Store, error) {
	store, err := r.Load()
	if err != nil {
		return Store{}, err
	}
	now := r.currentTime()
	for i := range store.Items {
		if store.Items[i].ID != id {
			continue
		}
		if store.Items[i].Status == StatusTodo && store.Items[i].CheckedAt == nil {
			store.Items[i].CheckedAt = &now
			store.Items[i].UpdatedAt = now
		} else {
			store.Items[i].Status = StatusTodo
			store.Items[i].Term = normalizeTerm(store.Items[i].Term)
			store.Items[i].CheckedAt = nil
			store.Items[i].DoneAt = nil
			store.Items[i].ArchivedAt = nil
			store.Items[i].Order = nextActiveOrderForTerm(store.Items, store.Items[i].Term)
			store.Items[i].UpdatedAt = now
		}
		return store, r.Save(store)
	}
	return store, nil
}

func (r *Repository) Edit(id string, text string) (Store, error) {
	store, err := r.Load()
	if err != nil {
		return Store{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Store{}, errors.New("todo text is required")
	}
	now := r.currentTime()
	for i := range store.Items {
		if store.Items[i].ID == id && store.Items[i].Status == StatusTodo && store.Items[i].CheckedAt == nil {
			store.Items[i].Text = text
			store.Items[i].UpdatedAt = now
			return store, r.Save(store)
		}
	}
	return store, nil
}

func (r *Repository) Move(id string, delta int) (Store, error) {
	store, err := r.Load()
	if err != nil {
		return Store{}, err
	}
	if MoveActive(&store, id, delta, r.currentTime()) {
		return store, r.Save(store)
	}
	return store, nil
}

func (r *Repository) MoveTerm(id string) (Store, error) {
	store, err := r.Load()
	if err != nil {
		return Store{}, err
	}
	if MoveActiveToOtherTerm(&store, id, r.currentTime()) {
		return store, r.Save(store)
	}
	return store, nil
}

func (r *Repository) currentTime() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func readStore(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{Version: SchemaVersion, Items: []Item{}}, nil
		}
		return Store{}, fmt.Errorf("read todos: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return Store{Version: SchemaVersion, Items: []Item{}}, nil
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, fmt.Errorf("decode todos: %w", err)
	}
	Normalize(&store)
	return store, nil
}

func readListsStore(path string, now time.Time) (ListsStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			catalog := ListsStore{Version: SchemaVersion, CurrentListID: DefaultListID}
			NormalizeLists(&catalog, now)
			return catalog, nil
		}
		return ListsStore{}, fmt.Errorf("read todo lists: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		catalog := ListsStore{Version: SchemaVersion, CurrentListID: DefaultListID}
		NormalizeLists(&catalog, now)
		return catalog, nil
	}
	var catalog ListsStore
	if err := json.Unmarshal(data, &catalog); err != nil {
		return ListsStore{}, fmt.Errorf("decode todo lists: %w", err)
	}
	NormalizeLists(&catalog, now)
	return catalog, nil
}

func Normalize(store *Store) {
	if store == nil {
		return
	}
	store.Version = SchemaVersion
	for i := range store.Items {
		if store.Items[i].Status == "" {
			store.Items[i].Status = StatusTodo
		}
		store.Items[i].Term = normalizeTerm(store.Items[i].Term)
		if store.Items[i].CreatedAt.IsZero() {
			store.Items[i].CreatedAt = store.Items[i].UpdatedAt
		}
		if store.Items[i].UpdatedAt.IsZero() {
			store.Items[i].UpdatedAt = store.Items[i].CreatedAt
		}
	}
	store.ArchiveMonths = normalizeArchiveMonths(append(store.ArchiveMonths, archiveMonthsFromItems(store.Items)...))
}

func NormalizeLists(catalog *ListsStore, now time.Time) {
	if catalog == nil {
		return
	}
	catalog.Version = SchemaVersion
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	byID := make(map[string]ListMeta, len(catalog.Lists)+1)
	for _, list := range catalog.Lists {
		id := NormalizeListID(list.ID)
		if id == "" {
			continue
		}
		name := NormalizeListName(list.Name)
		if name == "" {
			if id == DefaultListID {
				name = DefaultListName
			} else {
				name = id
			}
		}
		if id == DefaultListID {
			baseline := implicitDefaultListMeta()
			if list.CreatedAt.IsZero() {
				list.CreatedAt = baseline.CreatedAt
			}
			if list.UpdatedAt.IsZero() {
				list.UpdatedAt = baseline.UpdatedAt
			}
			if list.Rev <= 0 {
				list.Rev = baseline.Rev
			}
		} else {
			if list.CreatedAt.IsZero() {
				list.CreatedAt = now
			}
			if list.UpdatedAt.IsZero() {
				list.UpdatedAt = list.CreatedAt
			}
			if list.Rev <= 0 {
				list.Rev = revForTime(list.UpdatedAt)
			}
		}
		if list.DeletedAt != nil {
			deletedAt := list.DeletedAt.UTC()
			list.DeletedAt = &deletedAt
		}
		list.ID = id
		list.Name = name
		list.CreatedAt = list.CreatedAt.UTC()
		list.UpdatedAt = list.UpdatedAt.UTC()
		if existing, ok := byID[id]; ok && listMetaWins(existing, list) {
			continue
		}
		byID[id] = list
	}
	defaultMeta, ok := byID[DefaultListID]
	if !ok {
		defaultMeta = implicitDefaultListMeta()
	}
	baseline := implicitDefaultListMeta()
	defaultMeta.ID = DefaultListID
	defaultMeta.Deleted = false
	defaultMeta.DeletedAt = nil
	if NormalizeListName(defaultMeta.Name) == "" {
		defaultMeta.Name = DefaultListName
	}
	if defaultMeta.CreatedAt.IsZero() {
		defaultMeta.CreatedAt = baseline.CreatedAt
	}
	if defaultMeta.UpdatedAt.IsZero() {
		defaultMeta.UpdatedAt = baseline.UpdatedAt
	}
	if defaultMeta.Rev <= 0 {
		defaultMeta.Rev = baseline.Rev
	}
	byID[DefaultListID] = defaultMeta

	active := make([]ListMeta, 0, len(byID))
	tombstones := make([]ListMeta, 0)
	for _, list := range byID {
		if list.ID == DefaultListID {
			continue
		}
		if list.Deleted {
			tombstones = append(tombstones, list)
			continue
		}
		active = append(active, list)
	}
	sort.SliceStable(active, func(i, j int) bool {
		left := strings.ToLower(active[i].Name)
		right := strings.ToLower(active[j].Name)
		if left != right {
			return left < right
		}
		if active[i].Name != active[j].Name {
			return active[i].Name < active[j].Name
		}
		return active[i].ID < active[j].ID
	})
	sort.SliceStable(tombstones, func(i, j int) bool {
		if tombstones[i].UpdatedAt.Equal(tombstones[j].UpdatedAt) {
			return tombstones[i].ID < tombstones[j].ID
		}
		return tombstones[i].UpdatedAt.After(tombstones[j].UpdatedAt)
	})
	lists := make([]ListMeta, 0, 1+len(active)+len(tombstones))
	lists = append(lists, byID[DefaultListID])
	lists = append(lists, active...)
	lists = append(lists, tombstones...)
	current := NormalizeListID(catalog.CurrentListID)
	if current == "" || !hasActiveList(ListsStore{Lists: lists}, current) {
		current = DefaultListID
	}
	catalog.CurrentListID = current
	catalog.Lists = lists
}

func NormalizeListName(name string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
}

func NormalizeListID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, "_", "-")
	var b strings.Builder
	prevDash := false
	for _, r := range id {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func SlugForListName(name string) string {
	id := NormalizeListID(name)
	if id == "" {
		return "list"
	}
	return id
}

func ActiveLists(catalog ListsStore) []ListMeta {
	NormalizeLists(&catalog, time.Time{})
	out := make([]ListMeta, 0, len(catalog.Lists))
	for _, list := range catalog.Lists {
		if !list.Deleted {
			out = append(out, list)
		}
	}
	return out
}

func ListByID(catalog ListsStore, id string) (ListMeta, bool) {
	id = NormalizeListID(id)
	for _, list := range catalog.Lists {
		if list.ID == id {
			return list, true
		}
	}
	return ListMeta{}, false
}

func MergeLists(local ListsStore, remote ListsStore, now time.Time) ListsStore {
	NormalizeLists(&local, now)
	NormalizeLists(&remote, now)
	byID := make(map[string]ListMeta, len(local.Lists)+len(remote.Lists))
	for _, list := range local.Lists {
		byID[list.ID] = list
	}
	for _, list := range remote.Lists {
		if existing, ok := byID[list.ID]; ok && listMetaWins(existing, list) {
			continue
		}
		byID[list.ID] = list
	}
	merged := ListsStore{Version: SchemaVersion, CurrentListID: local.CurrentListID, Lists: make([]ListMeta, 0, len(byID))}
	for _, list := range byID {
		merged.Lists = append(merged.Lists, list)
	}
	NormalizeLists(&merged, now)
	return merged
}

func Cleanup(store *Store, now time.Time) bool {
	if store == nil {
		return false
	}
	now = now.UTC()
	changed := false
	for i := range store.Items {
		item := &store.Items[i]
		if item.Status == StatusTodo && item.CheckedAt != nil && !now.Before(item.CheckedAt.Add(CheckedDelay)) {
			item.Status = StatusDone
			if item.DoneAt == nil {
				doneAt := item.CheckedAt.Add(CheckedDelay)
				item.DoneAt = &doneAt
			}
			item.UpdatedAt = now
			changed = true
		}
		if item.Status == StatusDone && item.DoneAt != nil && !now.Before(item.DoneAt.Add(DoneArchiveAfter)) {
			item.Status = StatusArchived
			archivedAt := now
			item.ArchivedAt = &archivedAt
			item.UpdatedAt = now
			changed = true
		}
	}
	return changed
}

func ActiveItems(store Store) []Item {
	items := filterItems(store.Items, func(item Item) bool {
		return item.Status == StatusTodo
	})
	sortActiveItems(items)
	return items
}

func ShortItems(store Store) []Item {
	return activeItemsByTerm(store, TermShort)
}

func LongItems(store Store) []Item {
	return activeItemsByTerm(store, TermLong)
}

func activeItemsByTerm(store Store, term string) []Item {
	items := filterItems(store.Items, func(item Item) bool {
		return item.Status == StatusTodo && normalizeTerm(item.Term) == term
	})
	sortActiveItems(items)
	return items
}

func sortActiveItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if termRank(items[i].Term) != termRank(items[j].Term) {
			return termRank(items[i].Term) < termRank(items[j].Term)
		}
		if items[i].CheckedAt == nil && items[j].CheckedAt != nil {
			return true
		}
		if items[i].CheckedAt != nil && items[j].CheckedAt == nil {
			return false
		}
		if items[i].Order != items[j].Order {
			return items[i].Order < items[j].Order
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
}

func termRank(term string) int {
	if normalizeTerm(term) == TermLong {
		return 1
	}
	return 0
}

func DoneItems(store Store) []Item {
	items := filterItems(store.Items, func(item Item) bool {
		return item.Status == StatusDone
	})
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].doneTime().After(items[j].doneTime())
	})
	return items
}

func ArchiveGroups(store Store) map[string][]Item {
	groups := map[string][]Item{}
	for _, item := range store.Items {
		if item.Status != StatusArchived || item.ArchivedAt == nil {
			continue
		}
		month := item.ArchivedAt.UTC().Format("2006-01")
		groups[month] = append(groups[month], item)
	}
	for month := range groups {
		items := groups[month]
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].archivedTime().After(items[j].archivedTime())
		})
		groups[month] = items
	}
	return groups
}

func ArchiveMonths(store Store) []string {
	groups := ArchiveGroups(store)
	months := make([]string, 0, len(groups)+len(store.ArchiveMonths))
	months = append(months, store.ArchiveMonths...)
	for month := range groups {
		months = append(months, month)
	}
	return normalizeArchiveMonths(months)
}

func ArchiveMonthItems(store Store, month string) []Item {
	return ArchiveGroups(store)[month]
}

func NonArchivedStore(store Store) Store {
	items := filterItems(store.Items, func(item Item) bool {
		return item.Status != StatusArchived
	})
	return Store{Version: SchemaVersion, Items: items}
}

func MergeArchiveMonth(store Store, month string, items []Item) Store {
	store.ArchiveMonths = normalizeArchiveMonths(append(store.ArchiveMonths, month))
	monthItems := make([]Item, 0, len(items))
	for _, item := range items {
		if item.Status != StatusArchived || item.ArchivedAt == nil || item.ArchivedAt.UTC().Format("2006-01") != month {
			continue
		}
		monthItems = append(monthItems, item)
	}
	if len(monthItems) == 0 {
		return store
	}
	out := Store{Version: SchemaVersion, Items: make([]Item, 0, len(store.Items)+len(items))}
	for _, item := range store.Items {
		if item.Status == StatusArchived && item.ArchivedAt != nil && item.ArchivedAt.UTC().Format("2006-01") == month {
			continue
		}
		out.Items = append(out.Items, item)
	}
	for _, item := range monthItems {
		out.Items = append(out.Items, item)
	}
	Normalize(&out)
	return out
}

func PreserveArchived(local Store, synced Store) Store {
	out := NonArchivedStore(synced)
	out.ArchiveMonths = append([]string(nil), local.ArchiveMonths...)
	syncedIDs := make(map[string]bool, len(out.Items))
	for _, item := range out.Items {
		syncedIDs[item.ID] = true
	}
	for _, item := range local.Items {
		if item.Status == StatusArchived && !syncedIDs[item.ID] {
			out.Items = append(out.Items, item)
		}
	}
	Normalize(&out)
	return out
}

func normalizeArchiveMonths(months []string) []string {
	seen := make(map[string]bool, len(months))
	out := make([]string, 0, len(months))
	for _, month := range months {
		month = strings.TrimSpace(month)
		if len(month) != len("2006-01") || seen[month] {
			continue
		}
		seen[month] = true
		out = append(out, month)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func archiveMonthsFromItems(items []Item) []string {
	months := make([]string, 0)
	for _, item := range items {
		if item.Status == StatusArchived && item.ArchivedAt != nil {
			months = append(months, item.ArchivedAt.UTC().Format("2006-01"))
		}
	}
	return months
}

func MoveActive(store *Store, id string, delta int, now time.Time) bool {
	if store == nil || delta == 0 {
		return false
	}
	term := ""
	for i := range store.Items {
		if store.Items[i].ID == id {
			term = normalizeTerm(store.Items[i].Term)
			break
		}
	}
	if term == "" {
		return false
	}
	active := make([]int, 0)
	for i := range store.Items {
		item := store.Items[i]
		if item.Status == StatusTodo && item.CheckedAt == nil && normalizeTerm(item.Term) == term {
			active = append(active, i)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		left := store.Items[active[i]]
		right := store.Items[active[j]]
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	pos := -1
	for i, index := range active {
		if store.Items[index].ID == id {
			pos = i
			break
		}
	}
	if pos < 0 {
		return false
	}
	target := pos + delta
	if target < 0 || target >= len(active) {
		return false
	}
	moved := active[pos]
	if delta > 0 {
		copy(active[pos:], active[pos+1:target+1])
	} else {
		copy(active[target+1:], active[target:pos])
	}
	active[target] = moved
	now = now.UTC()
	for order, index := range active {
		if store.Items[index].Order != order {
			store.Items[index].UpdatedAt = now
		}
		store.Items[index].Order = order
	}
	return true
}

func MoveActiveToOtherTerm(store *Store, id string, now time.Time) bool {
	if store == nil {
		return false
	}
	now = now.UTC()
	for i := range store.Items {
		item := &store.Items[i]
		if item.ID != id || item.Status != StatusTodo || item.CheckedAt != nil {
			continue
		}
		current := normalizeTerm(item.Term)
		next := TermLong
		if current == TermLong {
			next = TermShort
		}
		item.Term = next
		item.Order = nextActiveOrderForTerm(store.Items, next)
		item.UpdatedAt = now
		return true
	}
	return false
}

func nextActiveOrder(items []Item) int {
	return nextActiveOrderForTerm(items, TermShort)
}

func nextActiveOrderForTerm(items []Item, term string) int {
	term = normalizeTerm(term)
	maxOrder := -1
	for _, item := range items {
		if item.Status == StatusTodo && item.CheckedAt == nil && normalizeTerm(item.Term) == term && item.Order > maxOrder {
			maxOrder = item.Order
		}
	}
	return maxOrder + 1
}

func normalizeTerm(term string) string {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case TermLong:
		return TermLong
	default:
		return TermShort
	}
}

func (r *Repository) storePath(id string) string {
	if r == nil {
		return ""
	}
	id = NormalizeListID(id)
	if id == "" || id == DefaultListID {
		return r.path
	}
	return filepath.Join(r.baseDir, NamedListsDir, id+".json")
}

func defaultListMeta() ListMeta {
	return implicitDefaultListMeta()
}

func implicitDefaultListMeta() ListMeta {
	createdAt := defaultListBaselineTime()
	return ListMeta{
		ID:        DefaultListID,
		Name:      DefaultListName,
		Rev:       defaultListRev,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func defaultListBaselineTime() time.Time {
	return time.UnixMilli(defaultListRev).UTC()
}

func hasList(catalog ListsStore, id string) bool {
	id = NormalizeListID(id)
	for _, list := range catalog.Lists {
		if list.ID == id {
			return true
		}
	}
	return false
}

func hasActiveList(catalog ListsStore, id string) bool {
	id = NormalizeListID(id)
	for _, list := range catalog.Lists {
		if list.ID == id && !list.Deleted {
			return true
		}
	}
	return false
}

func listNameTaken(catalog ListsStore, name string, exceptID string) bool {
	name = strings.ToLower(NormalizeListName(name))
	exceptID = NormalizeListID(exceptID)
	for _, list := range catalog.Lists {
		if list.Deleted || list.ID == exceptID {
			continue
		}
		if strings.ToLower(NormalizeListName(list.Name)) == name {
			return true
		}
	}
	return false
}

func listMetaWins(current ListMeta, candidate ListMeta) bool {
	currentRev := listMetaRevision(current)
	candidateRev := listMetaRevision(candidate)
	if currentRev != candidateRev {
		return currentRev > candidateRev
	}
	if current.Deleted != candidate.Deleted {
		return current.Deleted
	}
	if !current.UpdatedAt.Equal(candidate.UpdatedAt) {
		return current.UpdatedAt.After(candidate.UpdatedAt)
	}
	return current.ID <= candidate.ID
}

func listMetaRevision(meta ListMeta) int64 {
	if meta.Rev > 0 {
		return meta.Rev
	}
	return revForTime(meta.UpdatedAt)
}

func revForTime(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().UnixMilli()
}

func (r *Repository) generateListID(catalog ListsStore) (string, error) {
	generate := randomListID
	if r != nil && r.newID != nil {
		generate = r.newID
	}
	for i := 0; i < 32; i++ {
		id, err := generate()
		if err != nil {
			return "", err
		}
		id = NormalizeListID(id)
		if id != "" && id != DefaultListID && !hasList(catalog, id) {
			return id, nil
		}
	}
	return "", errors.New("could not allocate unique todo list id")
}

func randomListID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate todo list id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}

func filterItems(items []Item, keep func(Item) bool) []Item {
	out := make([]Item, 0, len(items))
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}

func (i Item) doneTime() time.Time {
	if i.DoneAt != nil {
		return *i.DoneAt
	}
	return i.UpdatedAt
}

func (i Item) archivedTime() time.Time {
	if i.ArchivedAt != nil {
		return *i.ArchivedAt
	}
	return i.UpdatedAt
}

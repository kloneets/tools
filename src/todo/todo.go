package todo

import (
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
	path string
	now  func() time.Time
}

func NewRepository() *Repository {
	return NewRepositoryAt(DefaultPath())
}

func NewRepositoryAt(path string) *Repository {
	return &Repository{path: path, now: time.Now}
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

func (r *Repository) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

func (r *Repository) Load() (Store, error) {
	store, err := readStore(r.path)
	if err != nil {
		return Store{}, err
	}
	changed := Cleanup(&store, r.currentTime())
	if changed {
		if err := r.Save(store); err != nil {
			return Store{}, err
		}
	}
	return store, nil
}

func (r *Repository) Save(store Store) error {
	store.Version = SchemaVersion
	Normalize(&store)
	Cleanup(&store, r.currentTime())
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todos: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create todo directory: %w", err)
	}
	if err := os.WriteFile(r.path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write todos: %w", err)
	}
	return nil
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

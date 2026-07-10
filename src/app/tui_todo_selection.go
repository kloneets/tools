package app

import (
	"github.com/kloneets/tools/src/todo"
)

func (a *terminalApp) todoSelectableItems() []todo.Item {
	rows := a.todoSelectableRows()
	items := make([]todo.Item, 0, len(rows))
	for _, row := range rows {
		if row.item != nil {
			items = append(items, *row.item)
		}
	}
	return items
}

func (a *terminalApp) todoSelectableRows() []todoSelectableRow {
	rows := make([]todoSelectableRow, 0)
	for _, item := range todo.ShortItems(a.todoStore) {
		item := item
		rows = append(rows, todoSelectableRow{item: &item})
	}
	for _, item := range todo.LongItems(a.todoStore) {
		item := item
		rows = append(rows, todoSelectableRow{item: &item})
	}
	for _, item := range todo.DoneItems(a.todoStore) {
		item := item
		rows = append(rows, todoSelectableRow{item: &item})
	}
	for _, month := range todo.ArchiveMonths(a.todoStore) {
		rows = append(rows, todoSelectableRow{archiveMonth: month})
	}
	return rows
}

func (a *terminalApp) selectedTodoRow() (todoSelectableRow, bool) {
	rows := a.todoSelectableRows()
	if len(rows) == 0 {
		return todoSelectableRow{}, false
	}
	a.clampTodoIndex()
	return rows[a.todoIndex], true
}

func (a *terminalApp) selectedTodoItem() (todo.Item, bool) {
	row, ok := a.selectedTodoRow()
	if !ok || row.item == nil {
		return todo.Item{}, false
	}
	return *row.item, true
}

func (a *terminalApp) selectTodoByID(id string) {
	rows := a.todoSelectableRows()
	for i, row := range rows {
		if row.item != nil && row.item.ID == id {
			a.todoIndex = i
			return
		}
	}
	a.clampTodoIndex()
}

func (a *terminalApp) clampTodoIndex() {
	rows := a.todoSelectableRows()
	if len(rows) == 0 {
		a.todoIndex = 0
		return
	}
	if a.todoIndex < 0 {
		a.todoIndex = 0
	}
	if a.todoIndex >= len(rows) {
		a.todoIndex = len(rows) - 1
	}
}

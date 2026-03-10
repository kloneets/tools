# Lessons Learned

## Test Environment Checks
- Run `go test ./...` early, but expect sandboxed Go cache permission failures here and rerun with the approved `go test` escalation path.
- For GTK-backed tests, isolate with `go test ./src/ui -v -count=1 -timeout 30s` before assuming the whole suite is hanging.
- Use `gtk.InitCheck()` in tests and skip cleanly when GTK cannot initialize.

## Dependency and Toolchain Issues
- On Go 1.26, check `go4.org/unsafe/assume-no-moving-gc` immediately if GTK packages panic during test startup.
- Prefer fixing the dependency graph so plain `go test ./...` works, instead of relying on `ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH`.

## Repository-Specific Code Traps
- `google.golang.org/api/drive/v2` uses `MaxResults`, `items`, and `title`, not `PageSize`, `files`, and `name`.
- Password letter generation must include `z` and `Z`; watch loop bounds on character pools.
- Config directory creation should use `os.MkdirAll`, not `os.Mkdir`, because parent directories may be missing.
- With this `gotk4`/GTK tree stack, drag-and-drop experiments can cross into process-level crashes. If a sidebar/tree interaction can segfault, stop using that implementation and switch to a safer explicit interaction model.
- When implementing `Hide sidebar` for Notes, do not keep the action row inside the collapsible sidebar pane. Put the bottom buttons on a separate always-visible layer, and collapse the actual sidebar pane itself; otherwise the feature only hides content and leaves the sidebar width behind.
- Notes code preview must keep token colors separate from block styling. Do not set a foreground color on the code-block/base tag, or bright per-token colors collapse into one dull shade.
- When changing Notes appearance, verify the `Neon Burst` theme still shows visibly different colors for keywords, strings, comments, functions, properties, constants, types, and numbers.
- In Notes split-view scrolling, do not share the same GTK vertical adjustment object between editor and preview. It can break editor scrolling and contribute to layout instability; keep the editor scroller independent and synchronize preview scrolling explicitly instead.

## Testing Strategy
- Extract small pure helpers from GTK event handlers before writing tests.
- When asserting GTK container contents, prefer stable checks like child count or label text over pointer equality between wrapped objects.

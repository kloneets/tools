# Lessons Learned

## Test Environment Checks
- Run `go test ./...` early, but expect sandboxed Go cache permission failures here and rerun with the approved `go test` escalation path.

## Dependency and Toolchain Issues
- Prefer fixing the dependency graph so plain `go test ./...` works, instead of relying on `ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH`.

## Repository-Specific Code Traps
- `google.golang.org/api/drive/v2` uses `MaxResults`, `items`, and `title`, not `PageSize`, `files`, and `name`.
- Password letter generation must include `z` and `Z`; watch loop bounds on character pools.
- Config directory creation should use `os.MkdirAll`, not `os.Mkdir`, because parent directories may be missing.
- When implementing `Hide sidebar` for Notes, do not keep the action row inside the collapsible sidebar pane. Put the bottom buttons on a separate always-visible layer, and collapse the actual sidebar pane itself; otherwise the feature only hides content and leaves the sidebar width behind.
- Notes code preview must keep token colors separate from block styling. Do not set a foreground color on the code-block/base tag, or bright per-token colors collapse into one dull shade.
- When changing Notes appearance, verify the `Neon Burst` theme still shows visibly different colors for keywords, strings, comments, functions, properties, constants, types, and numbers.
- In the TUI, every new or changed key binding must update the help overlay in the same change. Do not add bindings without also updating `renderHelpOverlay()` and the short per-view help text.

## Testing Strategy
- Extract small pure helpers from event handlers before writing tests.

# ergen

`ergen` is an automated, lightning-fast Go error boilerplate generator. Utilizing Go's standard `go/ast` and `go/parser` engines, `ergen` parses files on/near the cursor, evaluates the parent function's return signature, and auto-injects clean `if err != nil` or `if !ok` blocks.

## Technical Architecture & Core Features

`ergen` evaluates target contexts using Go's standard library packages (`go/ast`, `go/parser`, `go/token`) in a deterministic pipeline:

- **AST-Based Cursor & Node Targeting**: Maps 1-based editor cursor coordinates to AST ranges. It matches the enclosing `ast.FuncDecl` and targets the relevant `ast.AssignStmt` (even if the cursor is placed inside multi-line calls or on the blank line immediately below).
- **Type-Aware Zero-Value Generation**: Traverses the outer function's `ast.FieldList` return signature. Non-error fields are dynamically resolved to their respective zero-value strings (e.g. `0`, `""`, `nil`, `Struct{}`).
- **Package-Level Type Lookup**: Parses all non-test files in the file's package directory (via `parser.ParseDir`) to build an AST type-definition registry. This allows resolving underlying basic type aliases (e.g., matching `type MyStatus int` to generate `MyStatus(0)` instead of `MyStatus{}`).
- **AST Spacing Normalization**: Resolves standard Go formatting anomalies. By rendering sub-expressions via `format.Node` with a `nil` or fresh `token.FileSet`, it strips old positional markers and forces parent-child layouts to align cleanly on a single line.
- **In-Memory Compilation Guard**: Re-runs `parser.ParseFile` inside the mutation pipeline on the formatted source in memory. The tool aborts and exits safely if formatting errors or AST corruptions are detected, ensuring original files are never damaged.

---

## Installation

Install `ergen` globally using Go's native distribution tool:

```bash
go install github.com/prathamanvekar/ergen@latest
```

Ensure your Go binary path (typically `$HOME/go/bin` or `$GOPATH/bin`) is included in your system's `PATH`.

---

## How `ergen` Compares

- **`gopls` / Editor Snippets**: Language server completion snippets provide static insertions (e.g., expanding `err` to `if err != nil`). They do not dynamically resolve the enclosing function's signature, necessitating manual edits to insert zero-values or align multi-value returns.
- **`koron/iferr`**: A classic utility designed for manual triggering, primarily generating standard `if err != nil` templates. It does not resolve custom package types, parse local directories, or support custom rules.
- **`motemen/go-iferr`**: Smartly evaluates function return parameters, but lacks a configuration schema to vary templates conditionally based on parameters or prefixes.

---

## Editor Integrations

### 1. Zed Editor
Add the task definition to your local task manager (`~/.zed/tasks.json` or `.zed/tasks.json`):
```json
[
  {
    "label": "Gen Smart Go Error",
    "command": "/home/pratham/go/bin/ergen",
    "args": [
      "-file",
      "$ZED_FILE",
      "-line",
      "$ZED_ROW"
    ],
    "use_new_terminal": false,
    "allow_concurrent_runs": true,
    "reveal": "never",
    "hide": "on_success",
    "reevaluate_context": true
  }
]
```

Add the following key mapping in (`~/.zed/keymap.json`):
```json
[
  {
    "context": "Editor && mode == full",
    "bindings": {
      "ctrl-shift-e": [
        "task::Spawn",
        {
          "task_name": "Gen Smart Go Error",
          "reevaluate_context": true
        }
      ]
    }
  }
]
```

---

### 2. Visual Studio Code (VS Code)
Create a global task in your global/user `tasks.json` file:
```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Gen Smart Go Error",
      "type": "shell",
      "command": "ergen",
      "args": [
        "-file",
        "${file}",
        "-line",
        "${lineNumber}"
      ],
      "presentation": {
        "reveal": "silent",
        "panel": "shared",
        "showReuseMessage": false,
        "clear": true
      },
      "problemMatcher": []
    }
  ]
}
```

Add the custom keybinding into your user `keybindings.json`:
```json
{
  "key": "ctrl+shift+e",
  "command": "workbench.action.tasks.runTask",
  "args": "Gen Smart Go Error",
  "when": "editorLangId == go"
}
```

---

### 3. Neovim (Lua Bindings)
Map the `ergen` execution asynchronously using Neovim's built-in Lua API. Add this key mapping configuration to your `init.lua`:

```lua
vim.keymap.set('n', '<leader>ee', function()
    local file = vim.fn.expand('%:p')
    local line = vim.fn.line('.')
    
    -- Execute ergen asynchronously to prevent UI freezing
    vim.fn.jobstart({ 'ergen', '-file', file, '-line', tostring(line) }, {
        on_exit = function(_, exit_code)
            if exit_code == 0 then
                -- Reload the active buffer silently to display the changes
                vim.cmd('silent! edit!')
            else
                vim.notify("ergen failed to write error block", vim.log.levels.WARN)
            end
        end
    })
end, { desc = "Auto-generate Go error block under cursor" })
```

---

## Configuration Architecture (`.errgen.json`)

To customize how code templates are generated, place an `.errgen.json` file in your project's root folder (the directory containing `go.mod`). Rules are processed sequentially from top to bottom (**First Match Wins**).

### Schema Specification
A configuration file consists of a top-level `"rules"` array. Each rule can declare one or more of the following filter fields (which are evaluated as logical `AND` statements):

| Field | Type | Description | Example |
| :--- | :--- | :--- | :--- |
| `name` | `string` | Unique identifier for logging and console output. | `"http_handler"` |
| `outer_has_param_type` | `string` | Matches if the parent function signature has a parameter of this type. | `"*testing.T"`, `"http.ResponseWriter"` |
| `outer_has_return_type`| `string` | Matches if the parent function return signature has a return value of this type. | `"error"` |
| `rhs_call_prefix` | `string` | Matches if the RHS assignment call starts with this prefix. | `"db.Query"`, `"os."` |
| `rhs_call_package` | `string` | Matches if the RHS call is from this package. | `"json"`, `"yaml"` |
| `rhs_call_name` | `string` | Matches if the RHS function has this exact name. | `"Unmarshal"`, `"Open"` |
| `template` | `string` | The error-handling code block template to inject. Supports `{msg}` macro formatting. | See example below. |

### Complete `.errgen.json` Example
```json
{
  "rules": [
    {
      "name": "testing_handler",
      "outer_has_param_type": "*testing.T",
      "template": "if err != nil {\n\tt.Fatalf(\"{msg}: %v\", err)\n}"
    },
    {
      "name": "db_transaction_rollback",
      "rhs_call_prefix": "tx.Commit",
      "template": "if err != nil {\n\t_ = tx.Rollback()\n\treturn fmt.Errorf(\"{msg}: %w\", err)\n}"
    },
    {
      "name": "http_handler_response",
      "outer_has_param_type": "http.ResponseWriter",
      "template": "if err != nil {\n\thttp.Error(w, \"Internal Server Error\", http.StatusInternalServerError)\n\treturn\n}"
    }
  ]
}
```

# ergen

`ergen` is an automated, lightning-fast Go error boilerplate generator. Utilizing Go's standard `go/ast` and `go/parser` engines, `ergen` parses files on/near the cursor, evaluates the parent function's return signature, and auto-injects clean `if err != nil` or `if !ok` blocks.

## Core Features & Implementation Details

- **Sub-3ms Local Static Analysis**: Operates deterministically via pure Go AST parsing. With no network roundtrips, API tokens, or inference overhead, completions generate locally in under 3 milliseconds.
- **Context-Aware Zero-Value Generation**: Iterates over the enclosing function's result parameters and recursively maps basic types, pointers, maps, channels, interfaces, and slices to their correct Go zero-values. It parses non-test files in the active package directory to dynamically distinguish between custom struct definitions (e.g. `MyStruct{}`) and custom primitive aliases (e.g. `MyStatus(0)`).
- **AST Integrity Safeguards**: Mutated source buffers are parsed and checked in memory using standard AST compilation passes before being committed to disk. If syntax verification fails, the operation aborts immediately, protecting original files from corruption.
- **Silent IDE Pipeline**: Exits gracefully with code `0` on expected out-of-scope targets (e.g., cursor not placed on or near an assignment statement) to ensure integrated terminal panes remain silent and hidden.

---

## Comparison with Existing Alternatives

| Feature / Capability | `ergen` | `gopls` / Editor Snippets | `koron/iferr` | `motemen/go-iferr` |
| :--- | :--- | :--- | :--- | :--- |
| **Execution Latency** | Sub-3ms | Instant | Sub-10ms | Sub-10ms |
| **Custom Rule Matching** | Yes (`.errgen.json`) | No (Static templates) | No | No |
| **Underlying Type Resolution** | Yes (Recursively maps packages) | No | No | Limited |
| **Compiler AST Safety Scans** | Yes (In-memory pre-check) | No | No | No |
| **Specialized LHS Variable Types** | Yes (Adapts to `err` or `ok` variables) | No | No | No |
| **Custom Logging/Tx Rollbacks** | Yes (Supports custom handler macros) | No | No | No |

### How `ergen` Compares:
1. **`gopls` / Editor Snippets**: Language server snippets are static code completions (e.g., expanding `err` to `if err != nil { return nil, err }`). They do not resolve the enclosing function's signature, leading to immediate syntax errors if the function returns more or fewer values, or expects non-nil custom type layouts.
2. **`koron/iferr`**: A classic Vim-compatible utility that parses local positions. However, it cannot resolve custom named types, does not verify code safety before writing, and lacks a conditional rules engine to vary templates dynamically.
3. **`motemen/go-iferr`**: Resolves basic Go function return parameters, but lacks a centralized configuration schema (`.errgen.json`) to enforce project-wide rules (such as running database rollbacks, log wrappers, or testing assertions).

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

### Rule Template Macro Formatting
The `template` string supports the `{msg}` macro, which is dynamically replaced with a clean camelCase or snake_case conversion of the called function name (e.g., `os.Open` generates the message `"failed to open"`). 

If `ergen` detects a non-error check variable on the LHS (e.g., `value, ok := assertion.(Type)`), it automatically translates standard template conditions from `err != nil` to `!ok` and replaces parameter error variables with corresponding custom fallback descriptions.

---

## Installation

Install `ergen` globally using Go's native distribution tool:

```bash
go install github.com/prathamanvekar/ergen@latest
```

Ensure your Go binary path (typically `$HOME/go/bin` or `$GOPATH/bin`) is included in your system's `PATH`.

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

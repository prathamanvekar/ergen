# ergen

`ergen` is an automated, lightning-fast Go error boilerplate generator. Utilizing Go's standard `go/ast` and `go/parser` engines, `ergen` parses files on/near the cursor, evaluates the parent function's return signature, and auto-injects clean `if err != nil` or `if !ok` blocks.

## Why `ergen`?

- **Zero-AI Latency (Sub-3ms Execution)**: Unlike slow, flaky AI-based completions requiring network queries or heavy local models, `ergen` operates deterministically via static analysis. It generates error blocks instantly in less than 3 milliseconds.
- **Deep Zero-Value Resolution**: Automatically resolves primitives, pointers, channels, interfaces, arrays, maps, and even package-level custom types (aliases vs. structs) to their exact Go zero-value definitions.
- **Safety First**: Implements an in-memory compiler pre-write parse scan to prevent corrupting your source files. If the modified AST fails syntax verification, `ergen` aborts the operation safely.
- **Graceful Editor UX**: Exits cleanly with code `0` on expected target-not-found lines to keep editor panels completely silent.

---

## Installation

Install `ergen` globally to your `$GOPATH/bin` or `$HOME/go/bin` using Go's native package distributor:

```bash
go install github.com/prathamanvekar/ergen@latest
```

Ensure your Go bin directory is in your system's `PATH`.

---

## Editor Integrations

### 1. Zed Editor
Add this to your local task manager (`~/.zed/tasks.json` or `.zed/tasks.json`):
```json
[
  {
    "label": "Gen Smart Go Error",
    "command": "ergen",
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

Add the following to your custom keymaps (`~/.zed/keymap.json`):
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

Add the following custom keybinding mapping into your user `keybindings.json`:
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

## Configuration (`.errgen.json`)

To customize how code templates are generated, copy `.errgen.json.example` to `.errgen.json` in your project's root folder:

```json
{
  "rules": [
    {
      "name": "testing_handler",
      "outer_has_param_type": "*testing.T",
      "template": "if err != nil {\n\tt.Fatalf(\"{msg}: %v\", err)\n}"
    }
  ]
}
```
Rules are processed sequentially (**First Match Wins**).

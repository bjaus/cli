# Plugins

The framework supports extending your CLI with external plugins — executable files that become subcommands. Plugins can be written in any language and are discovered at runtime.

## Implementing Discoverer

Implement `Discoverer` on your command to enable plugin discovery:

```go
type App struct{}

func (a *App) Discover() ([]cli.Commander, error) {
    return cli.Discover(
        cli.WithDirs(cli.DefaultDirs("myapp")...),
        cli.WithPATH("myapp"),
    )
}
```

## Discovery Modes

### Directory-Based Discovery

Scan directories for plugin executables:

```go
cli.Discover(
    cli.WithDir("./plugins"),
    cli.WithDir("/usr/local/lib/myapp/plugins"),
)
```

Or use the shorthand:

```go
cli.Discover(
    cli.WithDirs("./plugins", "/usr/local/lib/myapp/plugins"),
)
```

Every executable file in the directory becomes a plugin. The filename is the command name.

### Default Directories

`DefaultDirs` returns conventional plugin paths in priority order:

```go
cli.Discover(
    cli.WithDirs(cli.DefaultDirs("myapp")...),
)
```

Returns:
1. `./myapp/plugins` — project-level (highest priority)
2. `~/.config/myapp/plugins` — user-level
3. `/etc/myapp/plugins` — system-level (Unix only)

### PATH-Based Discovery

Scan the system PATH for executables matching `<prefix>-<command>`:

```go
cli.Discover(
    cli.WithPATH("myapp"),
)
```

This finds:
- `myapp-deploy` → command "deploy"
- `myapp-build` → command "build"
- `myapp-test` → command "test"

PATH results have lower priority than directory results.

### Combined Discovery

Use both modes together:

```go
cli.Discover(
    cli.WithDirs(cli.DefaultDirs("myapp")...),
    cli.WithPATH("myapp"),
)
```

Priority order:
1. First directory in `WithDirs`
2. Second directory in `WithDirs`
3. ...
4. PATH executables (lowest)

## Plugin Metadata Protocol

When a plugin is discovered, the framework calls it with `--cli-info` to get optional metadata:

```bash
$ ./deploy --cli-info
{"name":"deploy","description":"Deploy to cloud","aliases":["d"]}
```

### PluginInfo Structure

```go
type PluginInfo struct {
    Name        string   `json:"name,omitempty"`        // override command name
    Description string   `json:"description,omitempty"` // help text
    Aliases     []string `json:"aliases,omitempty"`     // alternate names
}
```

All fields are optional. A plugin that doesn't support `--cli-info` still works — it just has no description in help output.

### Custom Info Flag

Override the info flag name:

```go
cli.Discover(
    cli.WithInfoFlag("--plugin-info"),
)
```

## Writing Plugins

Plugins can be written in any language. They receive arguments via command-line and communicate via stdin/stdout/stderr.

### Go Plugin

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    if len(os.Args) > 1 && os.Args[1] == "--cli-info" {
        json.NewEncoder(os.Stdout).Encode(map[string]any{
            "name":        "deploy",
            "description": "Deploy to cloud environments",
            "aliases":     []string{"d"},
        })
        return
    }

    // Regular plugin logic
    fmt.Println("Deploying...")
}
```

### Bash Plugin

```bash
#!/bin/bash

if [ "$1" = "--cli-info" ]; then
    echo '{"name":"deploy","description":"Deploy to cloud environments"}'
    exit 0
fi

# Regular plugin logic
echo "Deploying to $1..."
```

### Python Plugin

```python
#!/usr/bin/env python3
import json
import sys

if len(sys.argv) > 1 and sys.argv[1] == "--cli-info":
    print(json.dumps({
        "name": "deploy",
        "description": "Deploy to cloud environments",
        "aliases": ["d"]
    }))
    sys.exit(0)

# Regular plugin logic
print(f"Deploying to {sys.argv[1] if len(sys.argv) > 1 else 'default'}...")
```

## ExternalCommand

Discovered plugins are wrapped in `ExternalCommand`:

```go
type ExternalCommand struct {
    Path           string   // absolute path to executable
    Cmd            string   // command name
    Desc           string   // description
    CommandAliases []string // aliases
    Args           Args     // receives positional args
}
```

When `Run` is called:
- Arguments are passed to the executable
- stdin, stdout, stderr are wired to the parent process
- The exit code is returned as an error

## Priority Rules

When commands from different sources share a name:

| Source | Priority | Behavior on Collision |
|--------|----------|----------------------|
| Embedded fields | Highest | Error with Subcommander |
| Subcommander | High | Error with embedded |
| Discoverer | Low | Silently yields |

Plugins silently yield to built-in commands since users don't control plugin names.

## Enumerating All Subcommands

Get the merged list of all subcommands:

```go
subs, err := cli.AllSubcommands(cmd)
if err != nil {
    return err
}
for _, sub := range subs {
    fmt.Println(sub.(cli.Namer).Name())
}
```

This includes:
1. Embedded Commander fields
2. Subcommander interface results
3. Discoverer interface results (plugins)

## Complete Example

```go
type App struct {
    Verbose bool `flag:"verbose"`
}

func (a *App) Name() string        { return "myapp" }
func (a *App) Description() string { return "My application" }
func (a *App) Run(ctx context.Context) error { return cli.ErrShowHelp }

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &ServeCmd{},  // built-in
        &ConfigCmd{}, // built-in
    }
}

func (a *App) Discover() ([]cli.Commander, error) {
    return cli.Discover(
        cli.WithDirs(cli.DefaultDirs("myapp")...),
        cli.WithPATH("myapp"),
    )
}

func main() {
    cli.ExecuteAndExit(context.Background(), &App{}, os.Args)
}
```

With plugins installed:

```
~/.config/myapp/plugins/
├── deploy        # becomes "myapp deploy"
├── backup        # becomes "myapp backup"
└── analyze       # becomes "myapp analyze"

# Or on PATH:
/usr/local/bin/myapp-deploy  # becomes "myapp deploy"
```

Help output:

```
My application

Usage:
  myapp [command]

Commands:
  serve     Start the server
  config    Configuration commands
  deploy    Deploy to cloud environments
  backup    Backup data
  analyze   Analyze metrics

Use "myapp [command] --help" for more information.
```

## Plugin Installation

### Manual Installation

```bash
# Project-level
mkdir -p ./myapp/plugins
cp deploy ./myapp/plugins/

# User-level
mkdir -p ~/.config/myapp/plugins
cp deploy ~/.config/myapp/plugins/

# System-level
sudo mkdir -p /etc/myapp/plugins
sudo cp deploy /etc/myapp/plugins/

# PATH-based
sudo cp myapp-deploy /usr/local/bin/
```

### Installation Script

```bash
#!/bin/bash
# install-plugin.sh

PLUGIN_NAME="deploy"
PLUGIN_URL="https://example.com/plugins/${PLUGIN_NAME}"
INSTALL_DIR="${HOME}/.config/myapp/plugins"

mkdir -p "${INSTALL_DIR}"
curl -fsSL "${PLUGIN_URL}" -o "${INSTALL_DIR}/${PLUGIN_NAME}"
chmod +x "${INSTALL_DIR}/${PLUGIN_NAME}"

echo "Installed ${PLUGIN_NAME} to ${INSTALL_DIR}"
```

## Testing Plugins

Test that your plugin responds to `--cli-info`:

```bash
$ ./my-plugin --cli-info | jq .
{
  "name": "my-plugin",
  "description": "Does something useful",
  "aliases": ["mp"]
}
```

Test integration with your CLI:

```bash
# Install to project plugins
cp my-plugin ./myapp/plugins/

# Verify discovery
myapp --help | grep my-plugin

# Run the plugin
myapp my-plugin arg1 arg2
```

## What's Next

- [Subcommands](subcommands.md) — Command hierarchies
- [Completion](completion.md) — Shell completions include plugins
- [Help](help.md) — Plugins appear in help output

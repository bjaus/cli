# Shell Completion

The framework provides shell completion for bash, zsh, fish, and PowerShell. Completions are generated at runtime via the `__complete` protocol, ensuring they're always up to date with the current command tree — including plugins.

## Generating Completion Scripts

### Using the Completion Command

Add the built-in completion command to your app:

```go
import "github.com/bjaus/cli/completion"

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &ServeCmd{},
        completion.Command(a, "myapp", os.Stdout),
    }
}
```

Users can then generate scripts:

```bash
# Bash
myapp completion bash > /etc/bash_completion.d/myapp

# Zsh
myapp completion zsh > "${fpath[1]}/_myapp"

# Fish
myapp completion fish > ~/.config/fish/completions/myapp.fish

# PowerShell
myapp completion powershell >> $PROFILE
```

### Direct Generation

Generate scripts directly in code:

```go
import "github.com/bjaus/cli/completion"

script := completion.Bash(root, "myapp")
script := completion.Zsh(root, "myapp")
script := completion.Fish(root, "myapp")
script := completion.PowerShell(root, "myapp")
```

## Built-in Completions

The framework provides completions automatically for:

### Subcommands

```
$ myapp <TAB>
serve     Start the server
config    Configuration commands
version   Show version
```

### Flags

```
$ myapp serve --<TAB>
--port      Port to listen on
--host      Host to bind to
--verbose   Enable verbose output
```

### Enum Values

Flags with `enum` tags complete their allowed values:

```go
type Cmd struct {
    Format string `flag:"format" enum:"json,yaml,text"`
}
```

```
$ myapp --format <TAB>
json   yaml   text
```

## Custom Completions

### Completer Interface

Implement `Completer` for dynamic completions:

```go
type DeployCmd struct {
    Env string `arg:"env"`
}

func (d *DeployCmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    // Fetch environments dynamically
    envs, _ := fetchEnvironments()
    return cli.Completions(envs...)
}
```

### Completions with Descriptions

Provide descriptions for each candidate:

```go
func (d *DeployCmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    return cli.CompletionsWithDesc(
        cli.Completion{Value: "us-east-1", Description: "N. Virginia"},
        cli.Completion{Value: "us-west-2", Description: "Oregon"},
        cli.Completion{Value: "eu-west-1", Description: "Ireland"},
    )
}
```

### FlagCompleter Interface

Complete specific flag values:

```go
type DeployCmd struct {
    Region string `flag:"region" help:"AWS region"`
    Size   string `flag:"size" help:"Instance size"`
}

func (d *DeployCmd) CompleteFlag(ctx context.Context, flag, value string) cli.CompletionResult {
    switch flag {
    case "region":
        return cli.Completions("us-east-1", "us-west-2", "eu-west-1")
    case "size":
        return cli.CompletionsWithDesc(
            cli.Completion{Value: "small", Description: "2 vCPU, 4GB RAM"},
            cli.Completion{Value: "medium", Description: "4 vCPU, 8GB RAM"},
            cli.Completion{Value: "large", Description: "8 vCPU, 16GB RAM"},
        )
    }
    return cli.NoCompletions()
}
```

### Type-Safe Flag References

Use `FlagNameFor` to avoid hardcoding flag names:

```go
func (d *DeployCmd) CompleteFlag(ctx context.Context, flag, value string) cli.CompletionResult {
    if flag == cli.FlagNameFor(d, &d.Region) {
        return cli.Completions("us-east-1", "us-west-2")
    }
    return cli.NoCompletions()
}
```

## Active Help

Display contextual hints during completion:

```go
func (d *DeployCmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    return cli.Completions("dev", "staging", "prod").
        WithActiveHelp("Select deployment environment")
}
```

```
$ deploy <TAB>
[Select deployment environment]
dev   staging   prod
```

Multiple messages can be added:

```go
return cli.Completions("dev", "staging", "prod").
    WithActiveHelp(
        "Select deployment environment",
        "Use 'prod' for production deployments",
    )
```

## Directives

Control shell behavior with directives:

```go
func (c *Cmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    return cli.NoCompletions().WithDirective(cli.ShellCompDirectiveFilterDirs)
}
```

| Directive | Description |
|-----------|-------------|
| `ShellCompDirectiveDefault` | Default behavior (file completion fallback) |
| `ShellCompDirectiveNoSpace` | Don't add space after completion |
| `ShellCompDirectiveNoFileComp` | Disable file completion |
| `ShellCompDirectiveError` | Indicate error occurred |
| `ShellCompDirectiveFilterFileExt` | Completions are file extensions to filter by |
| `ShellCompDirectiveFilterDirs` | Only complete directories |

### File Extension Filtering

Complete only specific file types:

```go
func (c *Cmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    return cli.Completions(".yaml", ".yml", ".json").
        WithDirective(cli.ShellCompDirectiveFilterFileExt)
}
```

```
$ config load <TAB>
config.yaml   config.json   settings.yml
```

### Directory Completion

Complete only directories:

```go
func (c *Cmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    return cli.NoCompletions().WithDirective(cli.ShellCompDirectiveFilterDirs)
}
```

## Completion Helpers

### Simple Completions

```go
// Values without descriptions
cli.Completions("foo", "bar", "baz")
```

### Completions with Descriptions

```go
// Values with descriptions
cli.CompletionsWithDesc(
    cli.Completion{Value: "dev", Description: "Development environment"},
    cli.Completion{Value: "prod", Description: "Production environment"},
)
```

### No Completions

Fall back to default shell behavior (file completion):

```go
cli.NoCompletions()
```

## Runtime Protocol

The framework uses the `__complete` protocol for runtime completion. When the shell calls:

```bash
myapp __complete serve --port 80
```

The framework:
1. Walks the command tree to find `serve`
2. Calls `Completer.Complete` if implemented
3. Falls back to flag/subcommand/enum completion
4. Outputs candidates with optional descriptions
5. Outputs a directive on the last line

Output format:
```
--port	Port to listen on
--host	Host to bind to
--verbose	Enable verbose output
:4
```

The `:4` is the directive (ShellCompDirectiveNoFileComp).

## Installation Instructions

### Bash

```bash
# System-wide (requires root)
myapp completion bash > /etc/bash_completion.d/myapp

# User-specific
myapp completion bash > ~/.local/share/bash-completion/completions/myapp

# Temporary (current session)
source <(myapp completion bash)
```

### Zsh

```bash
# Find your fpath
echo $fpath

# Install to a directory in fpath
myapp completion zsh > "${fpath[1]}/_myapp"

# Or create a completions directory
mkdir -p ~/.zsh/completions
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
myapp completion zsh > ~/.zsh/completions/_myapp

# Regenerate completion cache
rm -f ~/.zcompdump; compinit
```

### Fish

```bash
myapp completion fish > ~/.config/fish/completions/myapp.fish

# Or for system-wide
myapp completion fish | sudo tee /usr/share/fish/vendor_completions.d/myapp.fish
```

### PowerShell

```powershell
# Add to profile
myapp completion powershell >> $PROFILE

# Or source directly
myapp completion powershell | Out-String | Invoke-Expression
```

## Complete Example

```go
type DeployCmd struct {
    Env    string   `arg:"env" help:"Target environment"`
    Region string   `flag:"region" help:"AWS region"`
    Tags   []string `flag:"tag" help:"Resource tags"`
}

func (d *DeployCmd) Name() string { return "deploy" }

// Complete provides completions for the env argument
func (d *DeployCmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    // If we're completing the environment argument
    if len(args) == 0 || !strings.HasPrefix(args[len(args)-1], "-") {
        envs, err := fetchEnvironments(ctx)
        if err != nil {
            return cli.NoCompletions().WithDirective(cli.ShellCompDirectiveError)
        }
        return cli.Completions(envs...).
            WithActiveHelp("Select deployment environment")
    }
    return cli.NoCompletions()
}

// CompleteFlag provides completions for flag values
func (d *DeployCmd) CompleteFlag(ctx context.Context, flag, value string) cli.CompletionResult {
    switch flag {
    case "region":
        regions := []cli.Completion{
            {Value: "us-east-1", Description: "N. Virginia"},
            {Value: "us-west-2", Description: "Oregon"},
            {Value: "eu-west-1", Description: "Ireland"},
            {Value: "ap-northeast-1", Description: "Tokyo"},
        }
        return cli.CompletionsWithDesc(regions...)

    case "tag":
        // Common tag suggestions
        return cli.Completions("env=", "team=", "cost-center=").
            WithDirective(cli.ShellCompDirectiveNoSpace)
    }
    return cli.NoCompletions()
}
```

## What's Next

- [Configuration](configuration.md) — Config file integration
- [Errors](errors.md) — Error handling patterns
- [Plugins](plugins.md) — External plugin discovery

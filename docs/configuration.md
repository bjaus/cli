# Configuration

The framework supports loading flag values from external configuration files via `ConfigResolver`. Config values have lower priority than CLI args and environment variables, making them ideal for defaults.

## Priority Chain

Values are resolved in this order (highest to lowest):

1. **CLI argument** — `--port 8080`
2. **Environment variable** — `PORT=8080`
3. **Config resolver** — from config file
4. **Default tag** — `default:"8080"`
5. **Zero value** — `0` for int, `""` for string

## ConfigResolver

A `ConfigResolver` is a simple function:

```go
type ConfigResolver func(key ConfigKey) (value string, found bool)
```

Given a flag name, it returns the string value and whether it was found. The framework handles all type conversion, validation, and required checks.

### ConfigKey

The `ConfigKey` provides both the flag name and decomposed parts:

```go
type ConfigKey struct {
    Name  string   // full flag name (e.g., "db-host")
    Parts []string // decomposed parts (e.g., ["db", "host"])
}
```

Use `Parts` for resolvers backed by nested formats like YAML or JSON.

## Setting a Resolver

### Global Resolver

Set a resolver for all commands:

```go
f, _ := os.Open("config.json")
resolver, _ := config.FromJSON(f)

cli.Execute(ctx, root, args,
    cli.WithConfigResolver(resolver),
)
```

### Per-Command Resolver

Implement `ConfigProvider` for command-specific resolvers:

```go
type ServeCmd struct {
    ConfigPath string `flag:"config" help:"Config file path"`
    Port       int    `flag:"port" help:"Port to listen on"`
}

func (s *ServeCmd) ConfigResolver() cli.ConfigResolver {
    if s.ConfigPath == "" {
        return nil
    }
    f, err := os.Open(s.ConfigPath)
    if err != nil {
        return nil
    }
    resolver, _ := config.FromJSON(f)
    return resolver
}
```

The resolver is called **after** CLI args are parsed, so `--config` can specify the config file path dynamically.

## Built-in Resolvers

The `config` subpackage provides these resolvers:

### FromMap

Create a resolver from a string map:

```go
import "github.com/bjaus/cli/config"

resolver := config.FromMap(map[string]string{
    "port": "8080",
    "host": "localhost",
})
```

### FromJSON

Load a flat JSON object:

```go
f, _ := os.Open("config.json")
resolver, err := config.FromJSON(f)
```

Config file (`config.json`):
```json
{
  "port": "8080",
  "host": "localhost",
  "verbose": "true"
}
```

### FromEnvFile

Parse a `.env` file:

```go
f, _ := os.Open(".env")
resolver, err := config.FromEnvFile(f)
```

Config file (`.env`):
```env
PORT=8080
HOST=localhost
# Comments are supported
VERBOSE=true
export API_KEY="secret"  # export prefix works
```

Supported syntax:
- `KEY=VALUE` pairs
- Quoted values: `KEY="VALUE"` or `KEY='VALUE'`
- Comments: lines starting with `#` and inline comments
- `export` prefix: `export KEY=VALUE`

### Chain

Try multiple resolvers in priority order:

```go
resolver := config.Chain(
    localOverrides,   // highest priority
    projectConfig,    // project-level
    globalConfig,     // user defaults
)
```

The first resolver that finds a value wins.

## Custom Format Adapters

Since `FromMap` accepts any `map[string]string`, adding new formats is straightforward.

### YAML

```go
import "gopkg.in/yaml.v3"

func FromYAML(r io.Reader) (cli.ConfigResolver, error) {
    var m map[string]string
    if err := yaml.NewDecoder(r).Decode(&m); err != nil {
        return nil, err
    }
    return config.FromMap(m), nil
}
```

### TOML

```go
import "github.com/BurntSushi/toml"

func FromTOML(r io.Reader) (cli.ConfigResolver, error) {
    var m map[string]string
    if _, err := toml.NewDecoder(r).Decode(&m); err != nil {
        return nil, err
    }
    return config.FromMap(m), nil
}
```

### HCL

```go
import "github.com/hashicorp/hcl/v2"

func FromHCL(r io.Reader) (cli.ConfigResolver, error) {
    data, err := io.ReadAll(r)
    if err != nil {
        return nil, err
    }
    var m map[string]string
    if err := hcl.Unmarshal(data, &m); err != nil {
        return nil, err
    }
    return config.FromMap(m), nil
}
```

## Remote Configuration

For sources that don't map to files, implement `ConfigResolver` directly:

### Consul KV

```go
func FromConsul(client *consul.Client, prefix string) cli.ConfigResolver {
    return func(key cli.ConfigKey) (string, bool) {
        pair, _, err := client.KV().Get(prefix+"/"+key.Name, nil)
        if err != nil || pair == nil {
            return "", false
        }
        return string(pair.Value), true
    }
}
```

### AWS SSM Parameter Store

```go
func FromSSM(client *ssm.Client, prefix string) cli.ConfigResolver {
    return func(key cli.ConfigKey) (string, bool) {
        path := prefix + "/" + key.Name
        param, err := client.GetParameter(ctx, &ssm.GetParameterInput{
            Name:           &path,
            WithDecryption: aws.Bool(true),
        })
        if err != nil {
            return "", false
        }
        return *param.Parameter.Value, true
    }
}
```

### HashiCorp Vault

```go
func FromVault(client *vault.Client, path string) cli.ConfigResolver {
    return func(key cli.ConfigKey) (string, bool) {
        secret, err := client.Logical().Read(path)
        if err != nil || secret == nil {
            return "", false
        }
        if val, ok := secret.Data[key.Name].(string); ok {
            return val, true
        }
        return "", false
    }
}
```

## Nested Configuration

For nested config formats, use `ConfigKey.Parts`:

```go
// config.yaml:
// database:
//   host: localhost
//   port: 5432

type Cmd struct {
    DB DBConfig `prefix:"db-"`
}

type DBConfig struct {
    Host string `flag:"host"`
    Port int    `flag:"port"`
}
```

The flag `--db-host` has:
- `Name`: `"db-host"`
- `Parts`: `["db", "host"]`

Use `Parts` to navigate nested structures:

```go
func FromNestedYAML(data map[string]any) cli.ConfigResolver {
    return func(key cli.ConfigKey) (string, bool) {
        current := data
        for i, part := range key.Parts {
            if i == len(key.Parts)-1 {
                // Last part: get the value
                if val, ok := current[part].(string); ok {
                    return val, true
                }
                return "", false
            }
            // Navigate deeper
            if nested, ok := current[part].(map[string]any); ok {
                current = nested
            } else {
                return "", false
            }
        }
        return "", false
    }
}
```

## Complete Example

```go
type App struct {
    ConfigPath string `flag:"config" short:"c" help:"Config file path"`
    Verbose    bool   `flag:"verbose" short:"v" help:"Verbose output"`
}

func (a *App) ConfigResolver() cli.ConfigResolver {
    if a.ConfigPath == "" {
        return nil
    }

    f, err := os.Open(a.ConfigPath)
    if err != nil {
        return nil
    }
    defer f.Close()

    // Detect format from extension
    switch filepath.Ext(a.ConfigPath) {
    case ".json":
        resolver, _ := config.FromJSON(f)
        return resolver
    case ".yaml", ".yml":
        resolver, _ := FromYAML(f)
        return resolver
    case ".env":
        resolver, _ := config.FromEnvFile(f)
        return resolver
    default:
        return nil
    }
}

type ServeCmd struct {
    Port int    `flag:"port" default:"8080" help:"Port to listen on"`
    Host string `flag:"host" default:"localhost" help:"Host to bind to"`
}

func main() {
    cli.ExecuteAndExit(ctx, &App{}, os.Args)
}
```

Usage:
```bash
# Use CLI args (highest priority)
$ app serve --port 9000

# Use environment variable
$ PORT=3000 app serve

# Use config file
$ app --config config.json serve

# Layer all three (CLI wins over env wins over config)
$ PORT=3000 app --config config.json serve --port 9000
# Port = 9000 (CLI wins)
```

## Layered Configuration

Build a configuration hierarchy:

```go
func (a *App) ConfigResolver() cli.ConfigResolver {
    resolvers := []cli.ConfigResolver{}

    // Project config (highest priority)
    if f, err := os.Open(".app.yaml"); err == nil {
        if r, err := FromYAML(f); err == nil {
            resolvers = append(resolvers, r)
        }
        f.Close()
    }

    // User config
    home, _ := os.UserHomeDir()
    if f, err := os.Open(filepath.Join(home, ".config/app/config.yaml")); err == nil {
        if r, err := FromYAML(f); err == nil {
            resolvers = append(resolvers, r)
        }
        f.Close()
    }

    // System defaults (lowest priority)
    if f, err := os.Open("/etc/app/defaults.yaml"); err == nil {
        if r, err := FromYAML(f); err == nil {
            resolvers = append(resolvers, r)
        }
        f.Close()
    }

    if len(resolvers) == 0 {
        return nil
    }
    return config.Chain(resolvers...)
}
```

## What's Next

- [Flags](flags.md) — Flag parsing and environment variables
- [Lifecycle](lifecycle.md) — Setup and teardown hooks
- [Errors](errors.md) — Error handling patterns

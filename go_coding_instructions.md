# Concise Go Coding Guidelines for Cobra CLI Development

## I. General Go Language Guidelines

### A. Code Formatting

- Enforce `gofmt` for all Go code.
- Use `goimports` for import management and formatting.
  - Use `goimports -local <your_module_prefix>` (e.g., `goimports -local gitlab.com/gitlab-org`).

### B. Naming Conventions

- **Packages:** Short, concise, lowercase, single-word names. No underscores or mixedCaps. Match source directory base name. Avoid `import.`.
- **Variables, Functions, Methods:** Use `MixedCaps` or `mixedCaps`. Exported names start with an uppercase letter. Short names for small scopes (e.g., `i`, `r`, `buf`).
- **Getters:** No `Get` prefix (e.g., `owner` field's getter is `Owner()`).
- **Setters:** Use `Set` prefix (e.g., `SetOwner()`).
- **Interfaces:** Single-method interfaces: method name + "er" (e.g., `Reader`). Multi-method interfaces: descriptive name.
- **Errors:** Types: `<T>Error` (e.g., `type ExitError struct`). Values: `Err<T>` (e.g., `var ErrNotFound`).

### C. Documentation and Comments

- `godoc` comments for all exported names and packages.
- Doc comments immediately precede the declaration, no blank lines between.
- Comments are complete sentences.
- Package comment: First sentence starts "Package `packagename`...". One per multi-file package.
- Symbol comments: Start with the symbol name (e.g., "`MyFunction` does...").
- Use `godoc` syntax for headings, links, lists, code blocks. `gofmt` normalizes these.

### D. Error Handling

- Functions return `error` values, do not panic in library code.
- Check all returned `error` values explicitly.
- Wrap errors with context using `fmt.Errorf` and `%w` (Go 1.13+).
  - Error messages: explain _what_ failed, avoid "failed to", no capitalization or trailing punctuation.
- Define custom error types (implementing `error` and `Unwrap() error`) for specific error information.
- Use `errors.Is()` for sentinel errors, `errors.As()` for custom error types.
- Log errors at higher levels (e.g., `main` or command `RunE`), not in low-level functions.

### E. Project Structure and Go Modules

- Use Go Modules for all new projects.
- **Standard Project Layouts:**
  - `cmd/`: For `main` packages (e.g., `cmd/mycli/main.go`).
  - `internal/`: Private application code, not importable by other modules.
  - `pkg/`: Public library code, use sparingly.
  - Avoid `vendor/` unless specifically required.
- Avoid `util` or `misc` packages.

## II. Cobra Framework Guidelines

### A. Cobra Project Structure

- Minimal `main.go` in project root, calling `cmd.Execute()`.
- `cmd/` directory for all command definitions.
  - `root.go`: Defines the root command.
  - Subcommand files: Each subcommand in its own file (e.g., `cmd/version.go`).

### B. Command Definition

- **`cobra.Command` fields:**
  - `Use`: One-line usage (e.g., `add [item]`).
  - `Aliases`: Array of alternative command names.
  - `Short`: Brief description for help listings (under 50 chars if possible).
  - `Long`: Detailed description for `help <command>`.
  - `Example`: Practical usage examples.
- **Argument Validation (`Args`):** Use built-in validators (e.g., `cobra.ExactArgs(1)`).
- **Command Logic:** **Prefer `RunE` over `Run`** to return errors.
- **Subcommand Addition:** Use `parentCmd.AddCommand(childCmd)` in child's `init()`.

### C. `pflag` Flag Management

- Cobra uses `pflag` for POSIX-compliant flags.
- **Persistent Flags:** Available to the command and its subcommands. Defined with `cmd.PersistentFlags()`.
- **Local Flags:** Only for the specific command. Defined with `cmd.Flags()`.
- **Flag Naming:** Clear, descriptive long names (e.g., `--output-format`). Conventional single-character short names (e.g., `-o`).
- **Required Flags:** Mark with `cmd.MarkFlagRequired("flagname")` or `cmd.MarkPersistentFlagRequired("flagname")`.
- **Custom Flag Types:** Implement `pflag.Value` interface (`String()`, `Set(string)`, `Type()`).

### D. Configuration Management (with Viper)

- Use Viper for complex configuration.
- **Cobra Integration:**
  - Bind Cobra flags to Viper keys: `viper.BindPFlag("configkey", cmd.Flags().Lookup("flagname"))` in `init()`.
  - Load config in `cobra.OnInitialize(initConfig)` or `PersistentPreRunE`.
- **Configuration Sources & Precedence:** 1. Explicit `Set` -> 2. Flags -> 3. Env Vars -> 4. Config File -> 5. K/V Store -> 6. Defaults.
- **Config File Formats:** YAML, JSON, TOML, etc.. YAML/TOML preferred for human-editable files.
- **Config File Paths:** Use `viper.SetConfigName()`, `viper.SetConfigType()`, `viper.AddConfigPath()`.
  - **XDG:** Manually add XDG paths (e.g., `$XDG_CONFIG_HOME/yourAppName/config.yaml` or `$HOME/.config/yourAppName/config.yaml`) using `viper.AddConfigPath()`.
- **Environment Variables:** Use `viper.AutomaticEnv()`, `viper.SetEnvPrefix()`, `viper.SetEnvKeyReplacer()`.
- **Default Values:** Always set with `viper.SetDefault()`.

### E. Cobra Command Error Handling

- Use `RunE` for commands to return errors. Cobra handles printing and exit.
- Define CLI-specific custom error types.
- User-facing error messages (from `RunE` or `cmd.PrintErr()`) should be clear, concise, and actionable. Avoid internal details unless verbose/debug mode.
- Use meaningful exit codes (0 for success, 1 for general Cobra error, custom codes for specific app errors).

### F. Command Lifecycle Hooks

- Use `PersistentPreRunE`, `PreRunE`, `PostRunE`, `PersistentPostRunE` etc., for setup/teardown logic.
- Prefer `E` versions of hooks to allow error returns.
- `PersistentPreRunE`: Logic for a command and all its children (e.g., config loading, client init).
- `PreRunE`: Logic specific to a single command before `RunE`.

### G. Help and Usage Messages

- Cobra auto-generates help from `Use`, `Short`, `Long`, `Example` fields and flags.
- Customize help: `cmd.SetHelpCommand()`, `cmd.SetHelpFunc()`, `cmd.SetHelpTemplate()`.
- Customize usage: `cmd.SetUsageFunc()`, `cmd.SetUsageTemplate()`.
- `cmd.SilenceUsage = true`: Prevents auto-printing usage on `RunE` error.
- `cmd.SilenceErrors = true`: Prevents Cobra from printing `RunE` errors (handle manually).

## III. CLI Application Best Practices

### A. Input/Output (I/O) Management

- `os.Stdout` (`cmd.OutOrStdout()`): For primary, parsable output data.
- `os.Stderr` (`cmd.ErrOrStderr()`): For errors, logs, progress, diagnostics.
- Consider `--output <format>` (JSON, CSV) for machine-readable output.
- **TTY Detection:** Use `golang.org/x/term`'s `IsTerminal(fd int)` or `mattn/go-isatty` to adjust behavior (e.g., colors, interactive prompts).

### B. User Experience Enhancements

- **Progress Indicators (Bars, Spinners):** For long operations. Libraries: `vbauerster/mpb`, `pterm/pterm`, `schollz/progressbar`, `briandowns/spinner`.
  - Output to `stderr`. Disable if not a TTY.
- **Interactive Prompts:** For dynamic input. Libraries: `AlecAivazis/survey/v2`, `manifoldco/promptui`.
  - Use only if `stdin` is a TTY. Provide non-interactive alternatives (flags).
- **Color/Formatting:** Use sparingly for readability. Libraries: `fatih/color`, `termenv`.
  - Provide `--no-color` flag or respect `NO_COLOR` env var. Disable if not a TTY.

### C. Security Considerations

- **Input Validation:** Validate ALL user input (flags, args, env vars, config files). Use whitelisting, type checks, regex.
- **Command Injection Prevention:**
  - Use `exec.Command(name, arg1, arg2...)` with separate arguments. NEVER pass user input directly into a command string to be parsed by a shell.
  - Prefer Go native functions over external commands.
- **Sensitive Data:**
  - NO hardcoding secrets.
  - Use env vars or secret management systems for API keys, passwords.
  - Use masked input for passwords (`survey.Password`, `term.ReadPassword`).
  - DO NOT log sensitive data.
- **Dependency Security:** Keep dependencies updated. Use `govulncheck`.

### D. Testing Strategies

- **Unit Tests:** For `RunE` logic and helpers. Use Dependency Injection and mocks.
- **Integration Tests (Cobra Commands):**
  - Use `cmd.SetArgs()`, `cmd.Execute()`. Capture output with `cmd.SetOut()`, `cmd.SetErr()` using `bytes.Buffer`.
  - Assert output, errors, and side effects.
- **Integration Tests (Binary):** Use `os/exec` to run the compiled CLI.
- **`testscript`:** For script-based CLI interaction testing.
- **I/O Mocking:** `bytes.Buffer`, `strings.NewReader` for `stdin`/`stdout`/`stderr`.
- **Filesystem Mocking:** Use `spf13/afero` (`MemMapFs`) for tests involving file operations.
- **Table-Driven Tests:** For multiple input/output scenarios.
- **Test Coverage:** Aim high. Use `go test -coverprofile=coverage.out`.

## IV. Tooling and Automation

### A. Enforced Code Formatting

- Mandatory: `gofmt`.
- Mandatory: `goimports`.

### B. Linting with `golangci-lint`

- Use `golangci-lint` as a linter aggregator.
- Configure via `.golangci.yml` in project root.
  - Specify Go version.
  - Enable/disable linters.
  - Configure specific linter options.
  - Set up issue exclusions (e.g., for `_test.go` files).
- **Recommended Linters to Enable for CLI:**
  - Defaults: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`.
  - Also enable: `gosec` (security), `stylecheck`, `typecheck`, `bodyclose` (if HTTP calls), `errorlint`, `goconst`, `gocritic`.
- Integrate into CI/CD pipelines.

### C. Documentation Generation

- **API Docs (`godoc`):** Write `godoc`-compliant comments for all exported symbols and packages (See I.C).
  - View locally with `go doc` or `godoc` server.
- **CLI Manuals (Cobra):** Auto-generate from command definitions.
  - Markdown: `doc.GenMarkdownTree(rootCmd, "./docs")`.
    - Customize with `doc.GenMarkdownCustom()` (e.g., for Hugo frontmatter).
  - Man Pages: `doc.GenManTree(rootCmd, header, "./man")`.
    - Use `doc.GenManHeader` for metadata.
  - Disable auto-gen tag: `cmd.DisableAutoGenTag = true`.
- **Publishing (`pkgsite`):**
  - `pkg.go.dev` for public modules.
  - Run `pkgsite.` locally for preview.

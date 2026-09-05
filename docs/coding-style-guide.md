```{eval-rst}
:orphan:

.. meta::
   :description: Workshop Go coding style guide covering error handling, naming
                 conventions, code structure, testing patterns, and architecture
                 principles derived from PR discussions and contribution standards.
```

(coding_style_guide)=

# Workshop Go coding style guide

This style guide documents Go-specific coding conventions used in the Workshop project. It captures patterns from code review discussions in merged PRs and established project standards. These guidelines complement Canonical's general coding standards with Workshop-specific decisions.

The guide is evidence-based, derived from actual PR discussions between maintainers during code reviews.

---

## Error handling

**Error message format**

**Pattern**: Error messages start lowercase, contain no trailing punctuation, and follow the template: "what was attempted: why it went wrong".

**Exception**: Proper nouns (like "SDK", "LXD") may start with a capital letter.

**Rationale**: Maintains consistency with existing error handling patterns and provides clear, actionable user guidance.

**Good**:

```go
// From cmd/workshop/connect.go
return fmt.Errorf("cannot connect plugs and slots across different workshops")

// From cmd/workshop/list.go
return fmt.Errorf(`cannot list: "--project" incompatible with "--global"`)

// From internal/daemon/api_workshops.go
return statusBadRequest("project-id required")
```

**Avoid**:

```go
return fmt.Errorf("Cannot connect plugs.") // Starts with capital, has punctuation
return fmt.Errorf("Error") // Not descriptive enough
```


---

**Quoting values in messages**

**Pattern**: Interpolate dynamic values with `%q`. Wrap literal references (a fixed flag like `--global`, a command like `workshop list`, an enum keyword like `Ready`, an env var name like `SSH_AUTH_SOCK`, a JSON key, an attribute name) in double quotes inside a backtick raw string. Do not use single quotes for any kind of quoting in user-facing prose. Do not use `\"` escapes in double-quoted Go literals when the template contains literal double quotes; switch the delimiter to backticks instead. The same rules apply to Cobra `Short`/`Long`/`Example` strings and flag descriptions.

**Good**:

```go
// Dynamic value: %q quotes the interpolated identifier.
return fmt.Errorf("workshop name %q too long", file.Name)

// Literal flag names inside a template: backtick raw string with literal double quotes.
return fmt.Errorf(`cannot list: "--project" incompatible with "--global"`)

// Flag description with literal command references.
cmd.PersistentFlags().BoolVar(&c.WaitOnError, "wait-on-error", false,
    `Pause the operation on error; to resume, use "--continue" or "--abort".`)

// Wrapping an upstream error while quoting a literal config key.
return fmt.Errorf(`internal error: cannot read "forget" flag: %w`, err)
```

**Avoid**:

```go
// Escaped double quotes inside a double-quoted template.
return fmt.Errorf("cannot list: \"--project\" incompatible with \"--global\"")

// Single quotes around a literal keyword.
return fmt.Errorf("%s must be a map or one of the shortcuts 'true' or 'false'", context)

// Dynamic identifier rendered with %s instead of %q.
return fmt.Errorf("workshop %s not found", name)
```

**Rationale**: `%q` is backed by `strconv.Quote`, the same primitive as `strutil.Quoted`, so dynamic values render consistently across error messages, log output, and list helpers. Reserving backticks as Go raw-string delimiters and double quotes for both literals and `%q` output keeps every quoted artifact in the final message visually uniform. The same path and identifier double-quoting applies across Workshop's error messages.


---

**Error specificity**

**Pattern**: Return specific errors where possible to allow callers to handle them appropriately.

**Good**:

```go
if _, err := os.Stat(path); err != nil {
    if os.IsNotExist(err) {
        // Handle file not found specifically
        return fmt.Errorf("configuration file %q not found", path)
    }
    return fmt.Errorf("cannot access configuration file %q: %w", path, err)
}
```

**Avoid**:

```go
if _, err := os.Stat(path); err != nil {
    return fmt.Errorf("internal error") // Too generic, loses context
}
```

**Rationale**: Specific errors enable proper error handling and debugging. Avoid generic "internal error" wrappers unless implementation details must be hidden.


---

**Consistent error handling pattern**

**Pattern**: Use one of two standard patterns consistently throughout the codebase.

**For simple function calls**:

```go
if err := f(); err != nil {
    return err
}
```

**For functions with multiple returns**:

```go
val, err := f()
if err != nil {
    return err
}
```

**Examples from codebase**:

```go
// From cmd/workshopd/run.go
if err := dirs.CreateDirs(); err != nil {
    return err
}

// From cmd/sdk/main.go
if err := cmd.Execute(); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
```

**Rationale**: Consistent error handling improves code readability and maintainability.


---

**Explicit error checking**

**Pattern**: Always check and handle errors. Use `_ = someFunc()` to explicitly ignore intentionally.

**Good**:

```go
// Explicitly discarding error
_ = file.Close()

// Handling error
if err := file.Close(); err != nil {
    log.Printf("failed to close file: %v", err)
}
```

**Avoid**:

```go
file.Close() // Unchecked error
```

**Rationale**: The errcheck linter is enabled for new code to catch unhandled errors. Note that errcheck ignores certain common cases like `file.Close()` and a few others. Explicit `_` assignment shows intentional discard for other cases.


---

## Naming conventions

**Function names reflect behavior accurately**

**Pattern**: Function names should accurately describe what the function does. Use the "maybe" prefix for operations that are conditional or may not occur.

**Good**:

```go
// Returns a value if conversion is possible, otherwise returns false
func maybeFloatToInt(v float64) (int64, bool) {
    if _, frac := math.Modf(v); frac != 0 {
        return 0, false
    }
    return int64(v), true
}

// Conditionally presents warnings based on count and timestamp
func maybePresentWarnings(count int, timestamp time.Time) {
    if count == 0 {
        return
    }
    // ... present warnings
}

// Returns SDK installation if the device represents one, otherwise nil
func maybeSdkInstallation(key string, device map[string]string) (*workshop.SdkInstallation, error) {
    // Returns nil if device is not an SDK installation
}
```

**Examples from codebase**:

- `maybeRefresh()` - checks if refresh is needed
- `maybeBound()` - returns binding if one exists
- `maybePathError()` - wraps error as path error if applicable

**Rationale**: The "maybe" prefix is an established pattern in the codebase indicating conditional behavior, optional operations, or operations that may not apply in all cases.


---

**Descriptive variable names**

**Pattern**: Use names that reflect purpose or filtering intent, not generic permission terms.

**Good**:

```go
filters := []string{
    "config.user.workshop.project-id=" + pid,
    "config.user.workshop.name=" + w,
    "config.user.workshop.snapshot-type=" + kind,
}
snapshots, err := snapshotConn.GetInstancesWithFilter(api.InstanceTypeContainer, filters)
```

Here, `filters` clearly indicates these are filter conditions for querying snapshots via `GetInstancesWithFilter()`.

**Avoid**:

```go
conditions := []string{...}  // Not aligned with the method name
criteria := []string{...}    // Too generic; criteria for what?
```

**Rationale**: Improves code clarity and communicates intent. Use names that describe what the variable represents, not what it enables.


---

**Test constant naming**

**Pattern**: Test constants should have sensible, descriptive names that reflect their purpose.

**Good**:

```go
const (
    testProjectID   = "test-project-123"
    testWorkshopName = "dev-workshop"
    fakeAPIResponse = `{"status": "ready"}`
)
```

**Avoid**:

```go
const (
    s1 = "test-project-123"
    ws = "dev-workshop"
)
```

**Rationale**: Improves test readability and maintainability. In tests, variable names can be more relaxed, but global, reusable test constants should still be descriptive.


---

## Code structure and organization

**Complete related operations before moving to next attribute**

**Pattern**: When processing multiple attributes, finish all operations related to one attribute before moving to the next.

**Good**:

```go
// Process target attribute completely
target := plugAttrs["target"]
if err := validatePath(target); err != nil {
    return err
}
parsedTarget := parsePath(target)

// Now move to next attribute
source := plugAttrs["source"]
// ... process source
```

**Avoid**:

```go
// Getting target
target := plugAttrs["target"]

// Getting source
source := plugAttrs["source"]

// Validating target (separated from getting it)
if err := validatePath(target); err != nil {
    return err
}
```

**Rationale**: Improves code clarity by maintaining logical grouping of operations. Related code stays together.


---

**Extract common logic into reusable functions**

**Pattern**: Extract duplicated completion logic into reusable functions rather than duplicating inline.

**Good**:

```go
func completeWorkshopName(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    // Common workshop name completion logic
    return workshops, cobra.ShellCompDirectiveNoFileComp
}

cmd := &cobra.Command{
    ValidArgsFunction: completeWorkshopName,
}
```

**Avoid**:

```go
cmd := &cobra.Command{
    ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
        // Inline completion logic duplicated across commands
        cli, err := root.client()
        // ... 30 lines of logic ...
        return workshops, cobra.ShellCompDirectiveNoFileComp
    },
}
```

**Rationale**: Reduces duplication, avoids adding long inline functions into Cobra Command structure initialization, improves maintainability.


---

**Prefer existing attributes over reiterating**

**Pattern**: Use existing object attributes instead of reiterating collections when the information is already available.

**Good**:

```go
for _, plug := range plugs {
    if len(plug.Connections) > 0 {
        // Plug is connected
    }
}
```

**Avoid**:

```go
for _, plug := range plugs {
    isConnected := false
    for _, conn := range allConnections {
        if conn.Plug.Name == plug.Name {
            isConnected = true
            break
        }
    }
}
```

**Rationale**: Improves readability and reduces unnecessary iterations when data is already present in objects.


---

**Code should reflect logical flow clearly**

**Pattern**: Structure code to match its logical intent — filter first, then transform.

**Good**:

```go
// Filter connected plugs
var connectedPlugs []Plug
for _, plug := range allPlugs {
    if len(plug.Connections) > 0 {
        connectedPlugs = append(connectedPlugs, plug)
    }
}

// Transform to suggestions
for _, plug := range connectedPlugs {
    suggestions = append(suggestions, plug.ToCompletion())
}
```

**Avoid**:

```go
// Mixed filtering and transformation
for _, plug := range allPlugs {
    if len(plug.Connections) > 0 {
        suggestions = append(suggestions, plug.ToCompletion())
    }
}
```

**Rationale**: Makes intention explicit and improves readability for simple logic.

---

## Comments and documentation

**Comment format**

**Pattern**: Comments should be complete sentences starting with a capital letter and ending with a period.

**Good**:

```go
// Workshop represents a development environment running in a container.
type Workshop struct {
    Name string
    Base string
}

// validateName checks that the workshop name is valid.
func validateName(name string) error {
    // Empty names are not allowed.
    if name == "" {
        return fmt.Errorf("name cannot be empty")
    }
    return nil
}
```

**Avoid**:

```go
// workshop struct
type Workshop struct { ... }

// check name
func validateName(name string) error { ... }
```

**Rationale**: Proper comment formatting improves readability and maintains professional documentation standards.


---

**Godoc conventions**

**Pattern**: Exported functions and types must have Godoc comments. The comment should start with the name of the element.

**Good**:

```go
// Workshop represents a development environment.
type Workshop struct { ... }

// Launch creates and starts a new workshop with the given configuration.
func Launch(cfg *Config) (*Workshop, error) { ... }
```

**Avoid**:

```go
// Represents a development environment.
type Workshop struct { ... }

// Creates and starts a workshop.
func Launch(cfg *Config) (*Workshop, error) { ... }
```

**Rationale**: Following Godoc conventions ensures documentation is generated correctly and consistently.


---

## Type handling

**Use type switches for multiple possible types**

**Pattern**: When handling multiple possible input types, use type switches with explicit error messages for each case.

**Good**:

```go
switch ro := readOnly.(type) {
case bool:
    return ro, nil
case string:
    parsed, err := strconv.ParseBool(ro)
    if err != nil {
        return false, fmt.Errorf("invalid boolean string %q", ro)
    }
    return parsed, nil
default:
    return false, fmt.Errorf("read-only must be bool or string, got %T", ro)
}
```

**Avoid**:

```go
if b, ok := readOnly.(bool); ok {
    return b, nil
}
if s, ok := readOnly.(string); ok {
    // ... parse string
}
// No clear error for other types
```

**Rationale**: Provides better error reporting and makes code more maintainable. This is the established pattern used throughout the codebase for handling multiple types.

**Examples from codebase**:

```go
// From internal/asserts/constraint.go
switch x := v.(type) {
case string:
    return x, nil
case int:
    return strconv.Itoa(x), nil
default:
    return "", fmt.Errorf("invalid type %T", v)
}
```


---

**Avoid generics when concrete types are consistent**

**Pattern**: Don't use generics when type variation doesn't actually exist.

**Exception**: Test helpers and mock utilities may use generics to reduce code duplication across types.

**Good**:

```go
func filterByStatus(items []Workshop, status string) []Workshop {
    // Concrete types used consistently
}
```

**Avoid**:

```go
func filterByStatus[T any](items []T, status string) []T {
    // Unnecessary generics when T is always Workshop
}
```

**Rationale**: Simplifies code when type variation doesn't exist in practice.


---

## Testing patterns

**Test with JSON response mocking, not ad-hoc interfaces**

**Pattern**: In command packages, test API interactions by mocking JSON HTTP responses, not by creating ad-hoc client interfaces.

**Good**:

```go
// In cmd/workshop/connect_test.go
func (s *connectSuite) TestConnect(c *check.C) {
    s.RedirectClientToTestServer(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(ConnectionsResponse{
            Plugs: []Plug{{Name: "test"}},
        })
    })
    
    err := cmdConnect.Run(cmdConnect.Command(), []string{"test"})
    c.Assert(err, check.IsNil)
}
```

**Avoid**:

```go
// Creating ad-hoc interface in cmd package
type Client interface {
    Connections() ([]Connection, error)
}

func (c *CmdConnect) SetClient(cli Client) {
    c.client = cli
}
```

**Rationale**: Keeps interface definitions in appropriate packages (client library) and maintains architectural boundaries. Command packages should focus on CLI logic, not define their own client interfaces.


---

**Use real test data, not faked data**

**Pattern**: Tests should use realistic data structures that match actual API responses.

**Good**:

```go
testWorkshop := Workshop{
    Name:   "dev",
    Base:   "ubuntu@22.04",
    Status: "Ready",
    SDKs: []SDK{
        {Name: "go", Channel: "latest/stable"},
    },
}
```

**Avoid**:

```go
// Overly simplified fake data
testWorkshop := Workshop{
    Name: "test",
}
```

**Rationale**: Real data catches edge cases and integration issues that simplified fakes miss.


---

**Minimize duplication in test setup**

**Pattern**: Extract common test setup into helper functions or shared constants, but allow some duplication for clarity when needed.

**Good**:

```go
const readyWorkshopJSON = `{
    "name": "dev",
    "status": "Ready"
}`

func (s *testSuite) setupReadyWorkshop(c *check.C) Workshop {
    return Workshop{Name: "dev", Status: "Ready"}
}
```

**Avoid excessive coupling**:

```go
// Reusing status across unrelated tests
const sharedStatus = "Ready" // Used for both success and error cases
```

**Rationale**: Balance between DRY and test clarity. Some duplication is acceptable in tests to keep them self-contained and understandable.


---

## Architecture and separation of concerns

**Sorting belongs in representation layer, not client library**

**Pattern**: Client libraries should focus on data retrieval. Sorting and presentation logic belongs in the command/UI layer. Use the `slices` package for sorting.

**Good**:

```go
// In client library
func (c *Client) Changes() ([]Change, error) {
    // Just retrieve and return data
}

// In cmd package
func (c *CmdChanges) Run(cmd *cobra.Command, args []string) error {
    changes, err := c.client.Changes()
    if err != nil {
        return err
    }
    
    // Sort for presentation
    slices.SortFunc(changes, func(a, b Change) int {
        return cmp.Compare(b.ID, a.ID)
    })
}
```

**Avoid**:

```go
// In client library
func (c *Client) Changes() ([]Change, error) {
    changes, err := c.fetch()
    sort.Slice(changes, func(i, j int) bool {
        return changes[i].ID > changes[j].ID
    })
    return changes, err
}
```

**Rationale**: Separates data access from presentation concerns, making the client library reusable for different presentation needs.


---

**CLI command patterns**

**Pattern**: CLI commands should be transactional where possible and maintain consistent output formatting.

**Transactionality**:

```go
// Good: Use revert package for transactional operations
import "github.com/canonical/workshop/internal/revert"

func setupWorkshop(name string) error {
    r := revert.New()
    defer r.Fail()
    
    // Create container
    if err := createContainer(name); err != nil {
        return err
    }
    r.Add(func() { removeContainer(name) })
    
    // Install SDKs
    if err := installSDKs(name); err != nil {
        return err // Automatically reverts container creation
    }
    r.Add(func() { uninstallSDKs(name) })
    
    // Start workshop
    if err := startWorkshop(name); err != nil {
        return err // Automatically reverts everything
    }
    
    r.Success() // Mark as successful, skip revert
    return nil
}
```

**Alternative: Manual defer cleanup**:

```go
func setupMount(path string) (err error) {
    defer func() {
        if err != nil {
            // Clean up on error
            unmount(path)
        }
    }()
    
    if err := mount(path); err != nil {
        return err
    }
    
    if err := configure(path); err != nil {
        return err // defer will unmount
    }
    
    return nil
}
```

**Help strings**:

```go
// Good: Single spaces, concise
Short: "Launch a new workshop",
Long: `Launch creates and starts a workshop. The workshop will be based on the configuration in workshop.yaml.`,

// Avoid: Multiple spaces or verbose explanations
Short: "Launch  a  new  workshop",
Long: `This command will launch a new workshop. It will create the workshop based on the configuration...`,
```

**Output formatting**:

```go
// Good: Use tabwriter for consistent table formatting
import "github.com/juju/ansiterm/tabwriter"

func tabWriter() *tabwriter.Writer {
    return tabwriter.NewWriter(Stdout, 4, 3, 2, ' ', tabwriter.StripEscape)
}

func (c *CmdList) Run(cmd *cobra.Command, args []string) error {
    workshops, err := c.client.List()
    if err != nil {
        return err
    }
    
    w := tabWriter()
    fmt.Fprintf(w, "Name\tStatus\tBase\n")
    for _, ws := range workshops {
        fmt.Fprintf(w, "%s\t%s\t%s\n", ws.Name, ws.Status, ws.Base)
    }
    return w.Flush()
}
```

**Rationale**: Transactional commands prevent partial failures from leaving the system in an inconsistent state. Consistent formatting and output improves user experience.


---

## Nil handling patterns

**Use nil checks for "accept all" semantics**

**Pattern**: Use nil to represent "accept all" filtering, and extract into named functions for clarity.

**Good**:

```go
matchesStatus := func(s string) bool {
    if status == nil {
        return true // nil means accept all
    }
    return slices.Contains(status, s)
}

for _, workshop := range workshops {
    if matchesStatus(workshop.Status) {
        results = append(results, workshop)
    }
}
```

**Avoid**:

```go
for _, workshop := range workshops {
    if status == nil || slices.Contains(status, workshop.Status) {
        // Inline logic less clear
        results = append(results, workshop)
    }
}
```

**Rationale**: Makes the "accept all" intent explicit and improves readability.


---

## Code quality principles

**Blank lines for logical separation**

**Pattern**: Insert blank lines between logically different sections of code.

**Good**:

```go
func process() error {
    // Validation section
    if name == "" {
        return fmt.Errorf("name required")
    }
    if id == "" {
        return fmt.Errorf("id required")
    }

    // Data transformation section
    normalized := strings.ToLower(name)
    formatted := fmt.Sprintf("%s-%s", normalized, id)

    // Persistence section
    if err := save(formatted); err != nil {
        return err
    }

    return nil
}
```

**Rationale**: Improves code structure and makes it easier to understand different logical sections.


---

**Avoid nested conditions**

**Pattern**: Use early returns to reduce nesting levels.

**Good**:

```go
func validate(workshop *Workshop) error {
    if workshop == nil {
        return fmt.Errorf("workshop is nil")
    }
    
    if workshop.Name == "" {
        return fmt.Errorf("name required")
    }
    
    if !isValidBase(workshop.Base) {
        return fmt.Errorf("invalid base")
    }
    
    return nil
}
```

**Avoid**:

```go
func validate(workshop *Workshop) error {
    if workshop != nil {
        if workshop.Name != "" {
            if isValidBase(workshop.Base) {
                return nil
            } else {
                return fmt.Errorf("invalid base")
            }
        } else {
            return fmt.Errorf("name required")
        }
    } else {
        return fmt.Errorf("workshop is nil")
    }
}
```

**Rationale**: Reduces cognitive load, improves readability, and makes the happy path clearer. Use early returns or guard clauses to keep code flat and readable.


---

**Delete dead code and redundant comments**

**Pattern**: Remove unused code and comments that don't add value.

**Good**:

```go
func process() error {
    // Handle special case for empty input
    if input == "" {
        return nil
    }
    
    return transform(input)
}
```

**Avoid**:

```go
func process() error {
    // TODO: implement this later
    // Legacy code from old implementation
    // input := getOldInput()
    
    // Get input
    input := getInput()
    // Check if empty
    if input == "" {
        // Return nil
        return nil
    }
    
    // Transform the input
    return transform(input)
}
```

**Rationale**: Keeps codebase clean and maintainable. Redundant comments add noise without value.


---

**Normalize symmetries**

**Pattern**: Handle identical operations identically throughout the codebase.

**Good**:

```go
// Consistent error handling pattern everywhere
if err := validateName(name); err != nil {
    return err
}

if err := validateBase(base); err != nil {
    return err
}

if err := validateSDKs(sdks); err != nil {
    return err
}
```

**Avoid**:

```go
// Inconsistent handling
if err := validateName(name); err != nil {
    return err
}

err := validateBase(base)
if err != nil {
    return err
}

if validateSDKs(sdks) != nil {
    return validateSDKs(sdks) // Called twice!
}
```

**Rationale**: Consistency improves maintainability and reduces cognitive load when reading code. When the same operation appears in multiple places, it should be handled identically.


---

## Project-specific patterns

**Cobra command structure**

**Pattern**: Don't inline long functions in cobra.Command initialization. Extract ValidArgsFunction and RunE implementations.

**Good**:

```go
func newCmdConnect() *cobra.Command {
    c := &CmdConnect{}
    
    cmd := &cobra.Command{
        Use:               "connect",
        RunE:              c.Run,
        ValidArgsFunction: c.complete,
    }
    
    return cmd
}

func (c *CmdConnect) Run(cmd *cobra.Command, args []string) error {
    // Implementation
}

func (c *CmdConnect) complete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    // Completion implementation
}
```

**Avoid**:

```go
func newCmdConnect() *cobra.Command {
    cmd := &cobra.Command{
        Use: "connect",
        RunE: func(cmd *cobra.Command, args []string) error {
            // 50 lines of inline implementation
        },
        ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
            // 30 lines of inline completion logic
        },
    }
    
    return cmd
}
```

**Rationale**: Keeps command initialization clean and functions testable.


---

## Testing best practices

**Integration test patterns**

**Pattern**: Integration tests should test behavior, not internal implementation details.

**Good**:

```go
func (s *IntegrationSuite) TestLaunchWorkshop(c *check.C) {
    // Use c.Mkdir for automatic cleanup
    tmpDir := c.Mkdir()
    
    // Test the behavior
    err := s.cli.Launch("dev")
    c.Assert(err, check.IsNil)
    
    // Verify observable outcome
    workshops, err := s.cli.List()
    c.Assert(err, check.IsNil)
    c.Assert(workshops, check.HasLen, 1)
    c.Assert(workshops[0].Name, check.Equals, "dev")
}
```

**Avoid**:

```go
func (s *IntegrationSuite) TestLaunchWorkshop(c *check.C) {
    // Accessing internal state
    err := s.cli.Launch("dev")
    c.Assert(s.cli.internal.state.workshops["dev"].created, check.Equals, true)
}
```

**Best practices**:

- Use `c.Mkdir` from gocheck to create temporary directories that are automatically cleaned up
- Avoid relying on internal implementation details
- Test the behavior and observable outcomes
- Source documentation examples in tests to ensure documentation stays in sync with code

**Rationale**: Tests focused on behavior are more maintainable and less brittle when implementation changes.


---

**Unit test patterns**

**Pattern**: Use gocheck for unit tests, parameterize for edge cases, and avoid unnecessary mocks.

**Good**:

```go
func (s *ValidatorSuite) TestValidateName(c *check.C) {
    tests := []struct {
        name        string
        input       string
        expectedErr string
    }{
        {"valid name", "dev-workshop", ""},
        {"empty name", "", "name cannot be empty"},
        {"invalid chars", "dev@workshop", "invalid character"},
    }
    
    for _, tt := range tests {
        c.Logf("Testing: %s", tt.name)
        err := validateName(tt.input)
        if tt.expectedErr == "" {
            c.Assert(err, check.IsNil)
        } else {
            c.Assert(err, check.ErrorMatches, tt.expectedErr)
        }
    }
}
```

**Best practices**:
- Use gocheck for unit tests
- Parameterize tests to cover edge cases (different URL formats, empty inputs, boundary conditions)
- Avoid unnecessary mocks; prefer real lightweight implementations or fakes where feasible
- Use real test data that matches actual API responses

**Rationale**: Parameterized tests improve coverage, real data catches edge cases, and minimal mocking keeps tests maintainable.


---

## Internal package guidelines

**Visibility control**

**Pattern**: Keep types and functions unexported in `internal/` packages unless explicitly required by other packages.

**Good**:

```go
// internal/workshop/state.go

// workshopState is internal to this package
type workshopState struct {
    name   string
    status string
}

// Workshop is exported for use by other packages
type Workshop struct {
    Name   string
    Status string
}

// internal helper
func validateState(s *workshopState) error { ... }

// Exported API
func NewWorkshop(name string) (*Workshop, error) { ... }
```

**Avoid**:

```go
// Everything exported unnecessarily
type WorkshopState struct { ... }
func ValidateState(s *WorkshopState) error { ... }
```

**Rationale**: Reduces API surface area, makes refactoring easier, and prevents unintended coupling between packages.


---

**State management**

**Pattern**: The state package provides two distinct mechanisms: `state.Get()`/`state.Set()` for persistent data, and `state.Cache()` for transient caching.

**Persistent state with Get/Set**:

```go
import "github.com/canonical/workshop/internal/overlord/state"

// Store persistent data that survives restarts
func saveConnectionState(st *state.State) error {
    conns := map[string]any{
        "workshop/sdk:plug": "workshop/system:slot",
    }
    st.Set("conns", conns)
    return nil
}

// Retrieve persistent data
func loadConnectionState(st *state.State) (map[string]any, error) {
    var conns map[string]any
    err := st.Get("conns", &conns)
    if err != nil && err != state.ErrNoState {
        return nil, err
    }
    return conns, nil
}
```

**Transient caching with Cache**:

```go
// Cache objects for quick access within a session (not persisted)
func getStore(st *state.State) (*Store, error) {
    cached := st.Cached(cachedStoreKey{})
    if cached != nil {
        return cached.(*Store), nil
    }
    
    store := newStore()
    st.Cache(cachedStoreKey{}, store)
    return store, nil
}
```

**Avoid**:

```go
// Don't do manual JSON serialization for state
func saveState(path string, data any) error {
    json, _ := json.Marshal(data)
    return os.WriteFile(path, json, 0644)
}
```

**Important considerations**:
- `Get()`/`Set()` persist across restarts, serialized to JSON
- `Cache()` is for session-only data, cleared on restart
- Maps retrieved from state are references; modifications affect the original
- Always lock state before Get/Set/Cache operations

**Rationale**: State management APIs provide proper locking, change tracking, and persistence. Using them correctly avoids race conditions and ensures data consistency.


---

## Security considerations

**Script injection prevention**

**Pattern**: When generating scripts or templates, validate user input and use proper escaping mechanisms.

**Good**:

```go
import "github.com/canonical/workshop/internal/osutil"

func generateSetupScript(userInput string) (string, error) {
    // Validate input first using whitelist approach
    if err := validateScriptInput(userInput); err != nil {
        return "", err
    }
    
    // Use proper escaping for mount paths
    escaped := osutil.Escape(userInput)
    script := fmt.Sprintf("#!/bin/bash\nmount %s\n", escaped)
    return script, nil
}

func validateScriptInput(input string) error {
    // Whitelist approach for allowed characters
    if !regexp.MustCompile(`^[a-zA-Z0-9_/-]+$`).MatchString(input) {
        return fmt.Errorf("invalid characters in input")
    }
    return nil
}
```

**Available escaping utilities**:
- `osutil.Escape()` - Escapes paths for mount entries
- `osutil.Unescape()` - Unescapes mount entry paths
- Input validation before any script generation

**Avoid**:

```go
func generateSetupScript(userInput string) string {
    // Direct interpolation without validation or escaping
    return fmt.Sprintf("#!/bin/bash\nmount %s\n", userInput)
}
```

**Rationale**: Prevents script injection attacks when generating executable content from user input. Always validate and escape.


---

**File permissions**

**Pattern**: Be explicit about file permissions using appropriate constants or octal values.

**Good**:

```go
// Private file (owner read/write only)
if err := os.WriteFile(path, data, 0600); err != nil {
    return err
}

// Public read, owner write
if err := os.WriteFile(path, data, 0644); err != nil {
    return err
}

// Executable script
if err := os.WriteFile(scriptPath, data, 0755); err != nil {
    return err
}

// Using constant for standard permissions
if err := os.MkdirAll(dir, os.ModePerm); err != nil { // 0777
    return err
}

// Standard file creation (rw-rw-rw-), relying on umask
if err := os.WriteFile(path, data, 0666); err != nil {
    return err
}
```

**Avoid**:

```go
// Unclear permissions
if err := os.WriteFile(path, data, 0777); err != nil {
    return err
}

// Magic numbers without context
if err := os.WriteFile(path, data, 420); err != nil { // Decimal for 0644
    return err
}
```

**Rationale**: Explicit permissions ensure proper security boundaries and make intent clear.


---

## Common pitfalls and edge cases

**Map initialization**

**Pattern**: Always initialize maps before use. Writing to a nil map causes a panic.

**Good**:

```go
func newRegistry() *Registry {
    return &Registry{
        workshops: make(map[string]*Workshop),
        sdks:      make(map[string]*SDK),
    }
}

func addWorkshop(r *Registry, w *Workshop) {
    if r.workshops == nil {
        r.workshops = make(map[string]*Workshop)
    }
    r.workshops[w.Name] = w
}
```

**Avoid**:

```go
func addWorkshop(r *Registry, w *Workshop) {
    r.workshops[w.Name] = w // Panic if workshops is nil
}
```

**Rationale**: Prevents runtime panics from nil map writes.


---

**Loop variables in closures**

**Pattern**: Be careful with loop variables in closures, especially in goroutines.

**Good (Go 1.22+)**:

```go
for _, workshop := range workshops {
    go func() {
        // Safe in Go 1.22+: each iteration has its own workshop variable
        process(workshop)
    }()
}
```

**Good (Pre-Go 1.22 or explicit)**:

```go
for _, workshop := range workshops {
    workshop := workshop // Create loop-local copy
    go func() {
        process(workshop)
    }()
}
```

**Defer in loops**

**Pattern**: Be careful when using `defer` inside loops. Defers execute at function exit, not loop iteration exit.

**Good**:

```go
func processFiles(files []string) error {
    for _, filename := range files {
        if err := processFile(filename); err != nil {
            return err
        }
    }
    return nil
}

func processFile(filename string) error {
    f, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer f.Close() // Executes when processFile returns
    
    // Process file...
    return nil
}
```

**Avoid**:

```go
func processFiles(files []string) error {
    for _, filename := range files {
        f, err := os.Open(filename)
        if err != nil {
            return err
        }
        defer f.Close() // Won't execute until processFiles returns!
                        // May accumulate many open files
        
        // Process file...
    }
    return nil
}
```

**Rationale**: Defers in loops can cause resource leaks. Extract loop body into a function for proper cleanup.


---

## Contributing

When contributing code:

1. Follow the patterns documented in this guide
2. Run `golangci-lint run` before submitting code
3. Write tests for new functionality
4. Use `go test ./...` to verify tests pass
5. Keep changes focused and atomic
6. Write clear commit messages

For detailed contribution guidelines, see {ref}`Contribute to development <contributing_development>`.

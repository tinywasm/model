# Permitted Validation API

`Permitted` provides character-level whitelisting and length constraints for string validation. It is embedded in the `Field` struct and forms the basis of field-level validation rules.

## Overview

The `Permitted` type enables positive whitelisting — only explicitly enabled characters pass validation. This approach provides inherent XSS protection when used with standard widgets.

```go
type Permitted struct {
    Letters    bool       // a-z, A-Z, ñ, Ñ
    Tilde      bool       // á, é, í, ó, ú (and uppercase) — uses AL/AU from fmt/mapping.go
    Numbers    bool       // 0-9
    Spaces     bool       // ' '
    BreakLine  bool       // '\n'
    Tab        bool       // '\t'
    Extra      []rune     // additional allowed characters (e.g., '@', '.', '-')
    NotAllowed []string   // forbidden substrings
    Minimum    int        // min length (0 = no limit)
    Maximum    int        // max length (0 = no limit)
    StartWith  *Permitted // rules for first character (nil = same as main rules)
}
```

## Validation Rules

### Character Whitelisting

Only characters that match enabled flags are allowed. This is a **positive whitelist** — no character passes unless explicitly enabled.

| Flag | Characters |
|------|------------|
| `Letters` | `a-z`, `A-Z`, `ñ`, `Ñ` |
| `Tilde` | `á`, `à`, `ã`, `â`, `ä`, `é`, `è`, `ê`, `ë`, `í`, `ì`, `î`, `ï`, `ó`, `ò`, `õ`, `ô`, `ö`, `ú`, `ù`, `û`, `ü`, `ý` (and uppercase) |
| `Numbers` | `0-9` |
| `Spaces` | `' '` (space character) |
| `BreakLine` | `'\n'` (newline) |
| `Tab` | `'\t'` (tab) |

### Length Constraints

- `Minimum`: Enforced if > 0 (measured in runes, not bytes)
- `Maximum`: Enforced if > 0 (measured in runes, not bytes)

If only `Minimum`/`Maximum` are set (no character flags), validation only checks length and never rejects characters.

### Additional Rules

- `Extra`: Array of additional allowed characters (e.g., `[]rune{'@', '.', '-'}` for email-like strings)
- `NotAllowed`: Array of forbidden substrings (checked anywhere in the string)
- `StartWith`: Optional `Permitted` rules for the first character only; if `nil`, uses main rules

## Security Contract

**HTML-dangerous characters** (`<`, `>`, `&`, `"`, `'`) are **not included in any standard widget's whitelist**. This means data validated through standard widgets is **safe for HTML output** without additional escaping at the output layer.

**If custom code adds dangerous chars to `Extra`**, it must:
1. Document the XSS risk
2. Ensure proper output encoding (e.g., via `fmt.Convert(v).EscapeHTML()`)

## Methods

### Validate()

Checks that text conforms to the permitted rules. Validation order: length → forbidden substrings → start-with → characters.

```go
func (p Permitted) Validate(field, text string) error
```

Returns `nil` if valid, or an error describing the first validation failure.

### NoHTML()

Creates a copy with HTML-dangerous characters (`<`, `>`, `&`, `"`, `'`) added to `NotAllowed` as an explicit safety layer.

```go
p := model.Permitted{Letters: true, Extra: []rune{'@'}}.NoHTML()
// Now rejects: < > & " '
```

Use when custom `Extra` characters could appear in HTML injection attempts.

## Examples

### Simple text field

```go
model.Permitted{
    Letters: true,
    Spaces: true,
    Minimum: 1,
    Maximum: 100,
}
```

Allows: letters, spaces, 1-100 runes.

### Email-like string

```go
model.Permitted{
    Letters: true,
    Numbers: true,
    Extra: []rune{'@', '.', '-'},
    Minimum: 5,
}
```

Allows: letters, numbers, `@`, `.`, `-`, min 5 runes. Safe for HTML output.

### Number-only field

```go
model.Permitted{
    Numbers: true,
    Extra: []rune{'-', '+'},
    Minimum: 1,
    Maximum: 10,
}
```

Allows: digits, `-`, `+`, 1-10 runes. (Typical for phone or numeric ID).

### First character constraint (phone number)

```go
model.Permitted{
    Numbers: true,
    Extra: []rune{'-', '(', ')'},
    StartWith: &model.Permitted{
        Numbers: true, // First char must be a digit
    },
}
```

Allows: digits, `-`, `(`, `)` anywhere, but first char must be a digit.

### With forbidden substrings

```go
model.Permitted{
    Letters: true,
    Spaces: true,
    NotAllowed: []string{"admin", "root", "system"},
}
```

Allows: letters and spaces, but rejects if the string contains `"admin"`, `"root"`, or `"system"`.

## Integration with Field Validation

`Permitted` is embedded in `Field`:

```go
type Field struct {
    Name      string
    Type      FieldType
    NotNull   bool
    OmitEmpty bool
    Widget    Widget
    DB        *FieldDB
    Permitted          // embedded
}
```

When `Field.Validate(value)` is called:

1. **NotNull** check (if enabled)
2. **Widget.Validate()** call (if set)
3. **Length** check (via `Permitted.validateLength`)
4. **Character** and **substring** validation (via `Permitted.validateChars`)

The order ensures widget-level validation runs before character-level rules.

## Performance Considerations

- **Zero allocations:** Uses ASCII ranges and slice lookups, not maps or regex.
- **Early termination:** Stops at first validation failure.
- **Rune counting:** Uses range iteration (single pass) without importing `unicode/utf8`.

## See Also

- [Field and Fielder API](API_FIELD.md) for how `Permitted` integrates into schema validation
- [`tinywasm/fmt`](../../fmt/) for `Permitted` source and character mapping (AL/AU tables)

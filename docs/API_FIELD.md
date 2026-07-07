# Field and Fielder API

The `field` API provides a standard way to describe struct schemas, access their values, and perform validation without using runtime reflection. This is essential for maintaining a small binary size in WebAssembly environments.

**See also:** [`CODEC_AND_FIELDER.md`](CODEC_AND_FIELDER.md) for how `Field` relates to the typed codec (`Encodable`/`Decodable`).

## FieldType

`FieldType` represents the abstract storage type of a struct field.

| Constant | String | Description |
|----------|--------|-------------|
| `FieldText` | `"text"` | String or text data |
| `FieldInt` | `"int"` | Integer numbers |
| `FieldFloat` | `"float"` | Floating point numbers |
| `FieldBool` | `"bool"` | Boolean values |
| `FieldBlob` | `"blob"` | Binary data |
| `FieldStruct` | `"struct"` | Nested struct |
| `FieldIntSlice` | `"intslice"` | Slice of integers (`[]int`) |
| `FieldStructSlice` | `"structslice"` | Slice of nested structs (`[]Fielder`) |
| `FieldRaw` | `"raw"` | Pre-serialized JSON — emitted inline, no quoting |

### String()

Returns the string representation of the `FieldType`.

```go
model.FieldInt.String() // returns "int"
```

## Definition

`Definition` is the hand-written, fully-typed source of truth for a model. From a `Definition`, the `ormc` generator produces the concrete Go struct and its zero-reflection plumbing.

This inverts the previous flow (struct + string tags → generated schema): the schema literal is now authored by hand and everything else is derived.

```go
type Definition struct {
    Name   string // model identity: table name, ModelName(), route key
    Fields Fields // ordered schema; widgets come from any model.Widget (e.g. form/input)
}
```

### Fields Alias

`Fields` is an alias for `[]Field`, allowing literals to be assigned without conversion.

```go
type Fields = []Field
```

### Definition.Field()

Returns the field with the given name and `true`, or a zero `Field` and `false`.

```go
field, ok := UserModel.Field("email")
```

## Field

`Field` describes a single field in a struct's schema with its metadata, constraints, and validation rules.

```go
type Field struct {
    Name      string
    Type      FieldType
    NotNull   bool
    OmitEmpty bool        // omit from JSON when zero value
    Widget    Widget      // semantic input type; nil = no UI binding (set by ormc from `input:` tag)
    DB        *FieldDB    // nil for formonly/transport structs
    Ref       *Definition // composition OR scalar FK — disambiguated by Type, see below
    Exclude   bool        // field exists on the generated struct but is excluded from
                          // Pointers()/EncodeFields()/DecodeFields() — no persistence, no wire codec
    Permitted             // embedded: validation rules (characters, min/max)
}

type FieldDB struct {
    PK        bool
    Unique    bool
    AutoInc   bool
    RefColumn string // scalar FK only: column in Ref's table. Empty = auto-detect its PK.
    OnDelete  string // scalar FK only: ON DELETE action. Empty = generator default (e.g. CASCADE).
}
```

### Exclude

`Exclude: true` marks a field that must appear on the generated struct but must never be scanned,
persisted, or serialized by `ormc`-generated code — e.g. a password hash set via a side channel that
the struct needs to carry in memory but that has no business going through `Pointers()` or the codec.

```go
{Name: "password_hash", Type: model.FieldText, Exclude: true}
// ormc still generates: PasswordHash string
// but omits it from Pointers()/EncodeFields()/DecodeFields()
```

### Ref — two meanings, disambiguated by Type

`Ref *Definition` means one of two unrelated things depending on `Type` — never both at once:

- **`Type` is `FieldStruct`/`FieldStructSlice` → composition.** `Ref` is the nested `Definition`; the
  field's Go type comes from it. The nested value is part of THIS struct's own
  `Schema()`/`Pointers()`/codec (e.g. an embedded `Address` value).
- **`Type` is a scalar (`FieldText`/`FieldInt`/...) → scalar foreign key.** `Ref` is the `Definition`
  of the table this column references (e.g. `staff_id int64` pointing to `StaffModel`). It drives DDL
  foreign-key constraint generation (`orm.FieldExt`/`SchemaExt()`), using `FieldDB.RefColumn`/
  `FieldDB.OnDelete` for details. It does **not** change the field's Go type — that stays the plain
  scalar mapping from `Type`.

```go
// composition: Order embeds a ShippingAddress value
{Name: "shipping_address", Type: model.FieldStruct, Ref: &AddressModel}

// scalar FK: staff_id is an int64 column, but it's a foreign key into "staff"
{Name: "staff_id", Type: model.FieldInt, NotNull: true, Ref: &StaffModel,
    DB: &model.FieldDB{RefColumn: "id", OnDelete: "CASCADE"}}
```

**Known Go limitation — bidirectional Ref:** two package-level `Definition` vars cannot `Ref` each
other directly in their literals (Go rejects it as an "initialization cycle" — confirmed, not a
model design gap). A workaround exists (assign the pointers in `init()` instead of the literal), but
`ormc` is not expected to auto-detect or paper over this; if a bidirectional case is ever needed,
handle it explicitly.

### Type Mapping

`Field.Type` maps deterministically to a Go type. `ormc` uses this mapping to generate the concrete struct fields.

| `Field.Type` | Go Type |
|---|---|
| `FieldText`, `FieldRaw` | `string` |
| `FieldInt` | `int64` |
| `FieldFloat` | `float64` |
| `FieldBool` | `bool` |
| `FieldBlob` | `[]byte` |
| `FieldIntSlice` | `[]int` |
| `FieldStruct` | type of `Ref` (required) |
| `FieldStructSlice` | `[]` of type of `Ref` (required) |

## FieldDB

`FieldDB` contains database-specific metadata, extracted from `Field` to keep transport/UI structs lean. When `DB` is `nil`, the field has no database concerns (e.g., `formonly` structs).

```go
type FieldDB struct {
    PK      bool
    Unique  bool
    AutoInc bool
}
```

### Helper methods on Field

Convenience methods to avoid nil-check boilerplate:

```go
func (f Field) IsPK() bool      { return f.DB != nil && f.DB.PK }
func (f Field) IsUnique() bool   { return f.DB != nil && f.DB.Unique }
func (f Field) IsAutoInc() bool  { return f.DB != nil && f.DB.AutoInc }
```

## RawJSON Type

`RawJSON` is a type alias for `string` that signals pre-serialized JSON content. It tells the encoder to emit the field inline without quoting or re-serializing, following the pattern of `encoding/json.RawMessage` in the standard library.

```go
type Result struct {
    Content RawJSON            // no tag needed; type itself conveys intent
    Error   RawJSON `json:",omitempty"`
}
```

**Benefits:**

- **No linter warnings:** Using the type alias avoids the `unknown JSON option "raw"` error from placing `raw` in the standard `json:` tag.
- **Self-documenting:** The type clearly signals that the field contains pre-serialized JSON.
- **Zero overhead in WASM:** Type alias with `=` creates no runtime boxing — at compile time it's indistinguishable from `string`.

**How it works:**

1. The developer declares a field with type `RawJSON`
2. `ormc` code generation detects the type name and marks the field as `FieldRaw` in the Schema()
3. `tinywasm/json` encoder checks `Type == FieldRaw` and emits the value inline (no quoting or re-serializing)

## Widget Interface

`Widget` is the contract for a semantic input type. It is implemented by `tinywasm/form/input` types and by custom project inputs defined in `web/inputs/`. Set by `ormc` code generation from the `input:` struct tag.

```go
type Widget interface {
    Type() string                              // Semantic type name (e.g., "email", "textarea")
    Validate(value string) error               // Semantic validation for this input type
    Clone(parentID, name string) Widget        // Returns a positioned instance; pass ("","") for a bare template
}
```

`Field.Validate()` calls `Widget.Validate()` before `Permitted.Validate()`. If the widget fails, `Permitted` is not evaluated. If `Widget` is `nil`, only `Permitted` rules apply.

```go
// Example: field with Widget + additive Permitted rules
Field{
    Name:      "email",
    Widget:    input.NewEmail(),            // validates email format
    Permitted: model.Permitted{Minimum: 10}, // additionally enforces min length
}
```

**Why `Clone(parentID, name)` instead of `Clone()`:** The form layer always needs positioned instances for rendering. A parameterless `Clone()` would force a separate `Build()` method — two constructors for the same concern. With `Clone(parentID, name)`, consumers that only need a bare template pass `("", "")`, and form consumers get a positioned instance directly.

**Why not include `RenderHTML()`:** Interface Segregation — ORM and validation consumers need only `Type()` and `Validate()`. Rendering is handled by `tinywasm/form`, which type-asserts `field.Widget.Clone(parentID, name).(input.Input)` for HTML output.

### Validation (Permitted)

Validation rules are embedded in the `Field` via the `Permitted` struct. This includes character-level whitelisting, length constraints, and structural format checks.

| Field | Type | Description |
|-------|------|-------------|
| `Letters` | `bool` | Allows `a-z`, `A-Z`, `ñ`, `Ñ` |
| `Tilde` | `bool` | Allows accented characters (`á`, `é`, etc.) |
| `Numbers` | `bool` | Allows `0-9` |
| `Spaces` | `bool` | Allows `' '` |
| `BreakLine` | `bool` | Allows `\n` |
| `Tab` | `bool` | Allows `\t` |
| `Extra` | `[]rune` | Additional allowed characters |
| `NotAllowed` | `[]string`| Forbidden substrings |
| `Minimum` | `int` | Minimum length (runes) |
| `Maximum` | `int` | Maximum length (runes) |
| `StartWith` | `*Permitted`| Rules for the first character |

### Field.Validate()

Checks a string value against the field's constraints in order: `NotNull` → `Widget.Validate()` → `Permitted`.

```go
err := field.Validate("some value")
```

## Fielder Interface

The `Fielder` interface is the shared contract between various layers (like ORM and UI forms) to interact with structs.

```go
type Fielder interface {
    Schema() []Field
    Pointers() []any
}
```

### Contract

- `Schema()` and `Pointers()` MUST return slices of the same length.
- The i-th element in each slice corresponds to the same struct field.
- `Pointers()` returns pointers to fields for reading (dereference) and writing.

## FielderSlice Interface

`FielderSlice` is the interface used to read and append elements to a typed slice of structs without depending on runtime reflection. It embeds `Fielder`, allowing slice types to be passed to functions accepting `Fielder`.

```go
type FielderSlice interface {
    Fielder
    Len() int
    At(i int) Fielder
    Append() Fielder
}
```

This interface is implemented by generated code to allow iterators (like JSON encoding) to pull out nested structures reliably and with zero allocation footprint.

## Validator and SafeFields

```go
// Validator can self-validate
type Validator interface {
    Validate(action byte) error
}

// ModuleNaming provides a stable model identity
type ModuleNaming interface {
    ModelName() string
}

// Model describes a resource with a schema and a name.
type Model interface {
    Fielder
    ModuleNaming
}

// SafeFields combines Fielder and Validator
type SafeFields interface {
    Fielder
    Validator
}
```

### ValidateFields() Helper

Generic function that iterates through a `Fielder`'s schema and pointers to perform full validation based on the action. It handles `FieldText` (calling `Field.Validate`) and `FieldStruct` (recursive validation).

| Action | PK + AutoInc | PK without AutoInc | NotNull | Permitted |
|--------|--------------|--------------------|---------|-----------|
| `'c'` create | skip (DB assigns) | required | required | applies |
| `'u'` update | required | required | required | applies |
| `'d'` delete | required | required | skip | skip |
| other/unknown | required | required | required | applies |

```go
err := model.ValidateFields('u', myFielder)
```

## Conversion and Reading Helpers

### ReadValues()

For consumers that need a `[]any` of values (like ORM for SQL arguments):

```go
vals := model.ReadValues(myFielder.Schema(), myFielder.Pointers())
```

### ReadStringPtr()

High-performance codecs can read string values without boxing to `any`:

```go
if val, ok := model.ReadStringPtr(ptrs[i]); ok {
    // use val (string) directly
}
```

### IsZeroPtr()

Checks if a pointer points to its type's zero value.

```go
if model.IsZeroPtr(ptr, fieldType) {
    // value is zero
}
```

## Example Implementation

```go
type User struct {
    ID   string
    Name string
}

func (u *User) Schema() []model.Field {
    return []model.Field{
        {Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
        {Name: "name", Type: model.FieldText, NotNull: true, Permitted: model.Permitted{Letters: true}},
    }
}

func (u *User) Pointers() []any {
    return []any{&u.ID, &u.Name}
}

func (u *User) Validate(action byte) error {
    return model.ValidateFields(action, u)
}

func (u *User) ModelName() string {
    return "user"
}

func (u *User) EncodeFields(w model.FieldWriter) {
    w.String("id", u.ID)
    if u.Name != "" {
        w.String("name", u.Name)
    }
}

func (u *User) DecodeFields(r model.FieldReader) {
    if id, ok := r.String("id"); ok {
        u.ID = id
    }
    if name, ok := r.String("name"); ok {
        u.Name = name
    }
}

func (u *User) IsNil() bool {
    return u == nil
}
```

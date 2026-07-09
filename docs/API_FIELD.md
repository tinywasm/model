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

## Kind Interface

`Kind` replaces the old `FieldType` enum slot + `Widget` pair. It represents both the storage mapping and the semantic validation baseline. Implementations are stateless templates, safe for concurrent reuse.

```go
type Kind interface {
    Storage() FieldType          // deterministic Go/DDL mapping
    Name() string                // semantic name: "text", "int", "email", …
    Validate(value string) error // ALWAYS present — fail-closed
}
```

### Base Kinds

The `model` package provides constructors for base kinds covering every `FieldType`:

| Constructor | Storage | Validation Baseline |
|-------------|---------|---------------------|
| `Text()` | `FieldText` | digits, letters, spaces, common punctuation. Excludes `<>"'&`. |
| `Int()` | `FieldInt` | digits and optional leading `-`. |
| `Float()` | `FieldFloat` | digits, at most one `.`, and optional leading `-`. |
| `Bool()` | `FieldBool` | accepts `"true"`, `"false"`, `"1"`, `"0"`, `""`. |
| `Blob()` | `FieldBlob` | binary data (no content validation). |
| `Raw()` | `FieldRaw` | pre-serialized JSON (no content validation). |
| `Struct()` | `FieldStruct` | nested struct (no string validation). |
| `IntSlice()` | `FieldIntSlice` | slice of integers (no string validation). |
| `StructSlice()` | `FieldStructSlice` | slice of structs (no string validation). |

## Definition

`Definition` is the hand-written, fully-typed source of truth for a model. From a `Definition`, the `ormc` generator produces the concrete Go struct and its zero-reflection plumbing.

This inverts the previous flow (struct + string tags → generated schema): the schema literal is now authored by hand and everything else is derived.

```go
type Definition struct {
    Name   string // model identity: table name, ModelName(), route key
    Fields Fields // ordered schema; Kinds come from model constructors (e.g. model.Text())
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
    Type      Kind
    NotNull   bool
    OmitEmpty bool        // omit from JSON when zero value
    DB        *FieldDB    // nil for formonly/transport structs
    Ref       *Definition // composition OR scalar FK — disambiguated by Type.Storage(), see below
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

### Ref — two meanings, disambiguated by Type.Storage()

`Ref *Definition` means one of two unrelated things depending on `Type.Storage()` — never both at once:

- **`Storage()` is `FieldStruct`/`FieldStructSlice` → composition.** `Ref` is the nested `Definition`; the
  field's Go type comes from it. The nested value is part of THIS struct's own
  `Schema()`/`Pointers()`/codec (e.g. an embedded `Address` value).
- **`Storage()` is a scalar (`FieldText`/`FieldInt`/...) → scalar foreign key.** `Ref` is the `Definition`
  of the table this column references (e.g. `staff_id int64` pointing to `StaffModel`). It drives DDL
  foreign-key constraint generation (`orm.FieldExt`/`SchemaExt()`), using `FieldDB.RefColumn`/
  `FieldDB.OnDelete` for details. It does **not** change the field's Go type — that stays the plain
  scalar mapping from `Type`.

```go
// composition: Order embeds a ShippingAddress value
{Name: "shipping_address", Type: model.Struct(), Ref: &AddressModel}

// scalar FK: staff_id is an int64 column, but it's a foreign key into "staff"
{Name: "staff_id", Type: model.Int(), NotNull: true, Ref: &StaffModel,
    DB: &model.FieldDB{RefColumn: "id", OnDelete: "CASCADE"}}
```

**Known Go limitation — bidirectional Ref:** two package-level `Definition` vars cannot `Ref` each
other directly in their literals (Go rejects it as an "initialization cycle" — confirmed, not a
model design gap). A workaround exists (assign the pointers in `init()` instead of the literal), but
`ormc` is not expected to auto-detect or paper over this; if a bidirectional case is ever needed,
handle it explicitly.

### Type Mapping

`Field.Type.Storage()` maps deterministically to a Go type. `ormc` uses this mapping to generate the concrete struct fields.

| `Storage()` | Go Type |
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

Checks a string value against the field's constraints in order: `NotNull` → `Kind.Validate()` (baseline) → `Permitted` (length) → `Permitted` (chars).

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

#### Action Constants

The following `byte` constants should be used for the `action` parameter:

| Constant | Value | Description |
|----------|-------|-------------|
| `ActionCreate` | `'c'` | Model is being created (skips PK+AutoInc) |
| `ActionRead` | `'r'` | Model is being read (used for RBAC/tool convention) |
| `ActionUpdate` | `'u'` | Model is being updated (requires PK) |
| `ActionDelete` | `'d'` | Model is being deleted (only requires PK, skips other validation) |

#### Validation Matrix

| Action | PK + AutoInc | PK without AutoInc | NotNull | Permitted |
|--------|--------------|--------------------|---------|-----------|
| `ActionCreate` | skip (DB assigns) | required | required | applies |
| `ActionUpdate` | required | required | required | applies |
| `ActionDelete` | required | required | skip | skip |
| other/unknown | required | required | required | applies |

```go
err := model.ValidateFields(model.ActionUpdate, myFielder)
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
        {Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
        {Name: "name", Type: model.Text(), NotNull: true, Permitted: model.Permitted{Letters: true}},
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

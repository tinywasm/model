# Model Package Architecture

## Overview

The `model` package provides the foundational types and interfaces for the tinywasm ecosystem, establishing clear separation of concerns across schema, validation, and serialization.

## Responsibility Map

### tinywasm/model (This Package)

**Purpose:** Define the contract for schemas, validation, and typed serialization across all layers.

| Component | Responsibility | Used By |
|-----------|---|---|
| **Field, FieldType, FieldDB** | Schema metadata (DDL, types, constraints) | orm, form, json |
| **Fielder, FielderSlice** | Struct introspection without reflection | orm, json, form |
| **Permitted** | Character-level validation rules | field, form, json |
| **Validator, ValidateFields()** | Data integrity checking | orm, form, json |
| **Encodable, Decodable** | Typed serialization contract | json, jsvalue |
| **FieldWriter, FieldReader** | 0-alloc codec interface | json, jsvalue |
| **Widget** | Semantic input type contract | form (input generation) |
| **IDGenerator** | Identity-generation contract (mint a new PK) — no concrete generator hardcoded in a reusable module | domain modules (via injected `Deps`), unixid (implements it) |

### tinywasm/fmt (Refactored)

**Purpose:** String manipulation, type conversion, formatting, multilingual error handling.

| Component | Responsibility | Used By |
|-----------|---|---|
| **String operations** (Convert, Replace, Split, etc.) | Transformation and formatting | all packages |
| **Type conversion** (numbers, booleans, etc.) | Type-safe conversions | form, json, orm |
| **Error handling** (Err, Errf) | Standardized error messages | all packages |
| **Utilities** (HTML escape, JSON escape, etc.) | Format-specific escaping | json, dom |
| **Translation (lang subpackage)** | Multilingual error messages | all packages |

## Architecture Diagram

```
tinywasm/
  ├── model/               ← NEW: Schema & codec contracts
  │   ├── field.go         (moved from fmt)
  │   ├── permitted.go     (moved from fmt)
  │   ├── codec.go         (moved from fmt)
  │   └── docs/
  │       ├── API_FIELD.md
  │       ├── API_CODEC.md
  │       ├── CODEC_AND_FIELDER.md
  │       └── API_PERMITTED.md
  │
  ├── fmt/                 ← REFACTORED: String & format ops
  │   ├── convert.go
  │   ├── error.go
  │   ├── builder.go
  │   └── docs/
  │       ├── API_STRINGS.md
  │       ├── API_STRCONV.md
  │       └── MOVED_TO_MODEL.md
  │
  ├── orm/                 ← Uses Field, Fielder, Validate
  ├── json/                ← Uses Encodable, Decodable, Field
  ├── form/                ← Uses Field, Widget
  └── jsvalue/             ← Uses Encodable, Decodable
```

## Data Flow Examples

### Creating a Model (via ormc code generation)

```
User struct (Go)
  ↓ (ormc processes it)
  ├─→ Schema() []model.Field        (for orm DDL, validation, form)
  ├─→ Pointers() []any              (for orm SQL scanning)
  ├─→ Validate() error              (for orm, form, json validation)
  ├─→ EncodeFields(FieldWriter)     (for json, jsvalue serialization)
  └─→ DecodeFields(FieldReader)     (for json, jsvalue parsing)
```

### ORM Creating a Table

```
schema := user.Schema()
  ↓
For each Field in schema:
  ├─ Field.Name           → column name
  ├─ Field.Type           → column type (FieldInt → INTEGER, etc.)
  ├─ Field.DB.PK          → PRIMARY KEY constraint
  ├─ Field.DB.Unique      → UNIQUE constraint
  ├─ Field.DB.AutoInc     → AUTO_INCREMENT constraint
  └─ Field.NotNull        → NOT NULL constraint
```

### JSON Encoding

```
json.Encode(user)
  ↓
for range schema {
  encoder.FieldWriter.String("field_name", value)
}
  ↓
EncodeFields(w FieldWriter)     ← calls user-generated method
  ├─ w.String("id", user.ID)
  ├─ w.String("name", user.Name)
  └─ w.Int("age", int64(user.Age))
  ↓
JSON output
```

### Form Validation

```
form.Submit(userForm)
  ↓
model.ValidateFields('u', user)  ← validate for update action
  ↓
For each Field in schema:
  ├─ Field.NotNull check
  ├─ Field.Widget.Validate() check  (e.g., email format)
  └─ Field.Permitted check          (character whitelist)
  ↓
✅ Valid or ❌ Error
```

## Migration Guide (for consumers)

If you were importing from `tinywasm/fmt`:

```go
// OLD (v0.23 and earlier)
import "github.com/tinywasm/fmt"
field := fmt.Field{...}
codec := fmt.Encodable

// NEW (v0.24+)
import "github.com/tinywasm/model"
field := model.Field{...}
codec := model.Encodable
```

Both packages are still available. `fmt` continues to work but now focuses on string operations. Update imports to `model` for schema and codec types.

## Design Principles

1. **Single Responsibility** - Each package has one clear purpose
2. **Contracts Over Implementation** - Interfaces define behavior; generators (ormc) implement them
3. **Zero Dependencies** - No reflection, no maps, no runtime overhead in codec paths
4. **WASM-First** - Small binaries, minimal allocations, no stdlib imports
5. **One Schema Source** - `Field` serves all layers: DDL, validation, UI, serialization

## Related Packages

- [tinywasm/fmt](../fmt/) - String manipulation (moved Field/codec types to model)
- [tinywasm/orm](../orm/) - Uses Field, Fielder, ValidateFields
- [tinywasm/json](../json/) - Uses Encodable, Decodable, Field
- [tinywasm/form](../form/) - Uses Field, Widget, Permitted
- [tinywasm/jsvalue](../jsvalue/) - Uses Encodable, Decodable for JS boundary

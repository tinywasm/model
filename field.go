package model

import "github.com/tinywasm/fmt"

// FieldType represents the abstract storage type of a struct field.
type FieldType int

const (
	FieldText        FieldType = iota // Any string
	FieldInt                          // Any integer
	FieldFloat                        // Any float
	FieldBool                         // Boolean
	FieldBlob                         // Binary data ([]byte)
	FieldStruct                       // Nested struct (implements Fielder)
	FieldIntSlice                     // []int
	FieldStructSlice                  // []Fielder
	FieldRaw                          // Pre-serialized JSON — emitted inline, no quoting
)

var fieldTypeNames = []string{"text", "int", "float", "bool", "blob", "struct", "intslice", "structslice", "raw"}

// RawJSON is a type alias for string that signals pre-serialized JSON content.
// Like encoding/json.RawMessage, it tells the encoder to emit the value inline
// without quoting or re-serializing, avoiding linter warnings about the `json:",raw"` tag.
//
// Usage:
//
//	type Result struct {
//	    Content RawJSON            // no tag needed; type itself conveys intent
//	    Error   RawJSON `json:",omitempty"`
//	}
//
// The encoder detects RawJSON at code-generation time (ormc), marking the field
// as FieldRaw in the Schema(), and tinywasm/json handles it without re-serializing.
type RawJSON = string

func (ft FieldType) String() string {
	if int(ft) >= 0 && int(ft) < len(fieldTypeNames) {
		return fieldTypeNames[ft]
	}
	return "unknown"
}

// FieldDB contains database-specific metadata.
// Extracted from Field to keep transport/UI structs lean.
type FieldDB struct {
	PK      bool
	Unique  bool
	AutoInc bool
}

// Field describes a single field in a struct's schema.
// It provides type metadata, constraint flags, and validation rules
// used by database (orm), transport (json), UI (form), and validation layers.
//
// Validation rules are embedded via Permitted. When a field has validation
// configured, Field.Validate(value) checks the value against all rules.
// Fields without validation rules pass any value.
//
// Deterministic Field.Type → Go type mapping:
//
// | Field.Type | Go Type |
// |---|---|
// | FieldText, FieldRaw | string |
// | FieldInt | int64 |
// | FieldFloat | float64 |
// | FieldBool | bool |
// | FieldBlob | []byte |
// | FieldIntSlice | []int |
// | FieldStruct | Ref type (required) |
// | FieldStructSlice | [] of Ref type (required) |
type Field struct {
	Name      string
	Type      FieldType
	NotNull   bool
	OmitEmpty bool     // omit from JSON when zero value
	Widget    Widget   // ← NEW: set by ormc from `input:` tag. nil = no UI binding.
	DB        *FieldDB // nil for formonly/transport structs
	Ref       *Definition // only for FieldStruct / FieldStructSlice: points to the nested Definition.
	Permitted          // embedded: validation rules (characters, min/max)
}

// IsPK returns true if the field is a Primary Key.
func (f Field) IsPK() bool { return f.DB != nil && f.DB.PK }

// IsUnique returns true if the field has a Unique constraint.
func (f Field) IsUnique() bool { return f.DB != nil && f.DB.Unique }

// IsAutoInc returns true if the field is Auto-Increment.
func (f Field) IsAutoInc() bool { return f.DB != nil && f.DB.AutoInc }

// Validate checks a string value against this field's constraints.
// Checks NotNull first, then length, then delegates to Widget and Permitted.
//
// Security note: fields using standard widgets (Text, Textarea, Email) have whitelists
// that exclude HTML-dangerous characters. ValidateFields provides implicit XSS protection
// for form-submitted data. Data from external sources (DB reads, API responses) bypasses
// this check and must be encoded at the output layer.
func (f Field) Validate(value string) error {
	if f.NotNull && value == "" {
		return fmt.Err(f.Name, "required")
	}
	if value == "" {
		return nil // empty + not required = ok
	}

	if f.Widget != nil {
		if err := f.Widget.Validate(value); err != nil {
			return err
		}
	}

	// Always check length if configured, regardless of character rules
	if f.Minimum > 0 || f.Maximum > 0 {
		if err := f.Permitted.validateLength(f.Name, value); err != nil {
			return err
		}
	}

	// Only run character/substring validation if any char-rule is configured
	if f.hasPermittedRules() {
		return f.Permitted.validateChars(f.Name, value)
	}
	return nil
}

// hasPermittedRules returns true if any character-based Permitted field is non-zero.
func (f Field) hasPermittedRules() bool {
	return f.Letters || f.Tilde || f.Numbers || f.Spaces ||
		f.BreakLine || f.Tab || len(f.Extra) > 0 ||
		len(f.NotAllowed) > 0 || f.StartWith != nil
}

// ValidateFields validates all fields of a Fielder based on the action ('c', 'u', 'd').
// For each FieldText field, reads the string value and calls Field.Validate.
// For non-text fields with NotNull, checks against zero value.
//
// This is the single validation entry point — ormc-generated Validate()
// methods call this function first.
func ValidateFields(action byte, f Fielder) error {
	schema := f.Schema()
	ptrs := f.Pointers()
	for i, field := range schema {
		// 'd' delete: only PK required, skip everything else
		if action == 'd' {
			if field.IsPK() {
				switch field.Type {
				case FieldText, FieldRaw:
					val, _ := ReadStringPtr(ptrs[i])
					if val == "" {
						return fmt.Err(field.Name, "required")
					}
				default:
					if IsZeroPtr(ptrs[i], field.Type) {
						return fmt.Err(field.Name, "required")
					}
				}
			}
			continue
		}

		// 'c' create: skip PK+AutoInc (DB assigns it)
		if action == 'c' && field.IsPK() && field.IsAutoInc() {
			continue
		}

		switch field.Type {
		case FieldText, FieldRaw:
			val, _ := ReadStringPtr(ptrs[i])

			// PK always required (in 'c' without AutoInc, in 'u', and any other)
			if field.IsPK() && val == "" {
				return fmt.Err(field.Name, "required")
			}

			if err := field.Validate(val); err != nil {
				return err
			}

		case FieldStruct:
			// Recursive validation for nested structs
			if validator, ok := ptrs[i].(Validator); ok {
				if err := validator.Validate(action); err != nil {
					return err
				}
			} else if fielder, ok := ptrs[i].(Fielder); ok {
				if err := ValidateFields(action, fielder); err != nil {
					return err
				}
			}

		default:
			// PK always required
			if field.IsPK() && IsZeroPtr(ptrs[i], field.Type) {
				return fmt.Err(field.Name, "required")
			}
			// Non-text fields: only check NotNull (zero value check)
			if field.NotNull && IsZeroPtr(ptrs[i], field.Type) {
				return fmt.Err(field.Name, "required")
			}
		}
	}
	return nil
}

// IsZeroPtr checks if a pointer points to a zero value.
func IsZeroPtr(ptr any, ft FieldType) bool {
	switch ft {
	case FieldText, FieldRaw:
		if p, ok := ptr.(*string); ok && p != nil {
			return *p == ""
		}
	case FieldInt:
		switch p := ptr.(type) {
		case *int64:
			return *p == 0
		case *int:
			return *p == 0
		case *int32:
			return *p == 0
		case *uint:
			return *p == 0
		case *uint32:
			return *p == 0
		case *uint64:
			return *p == 0
		}
	case FieldFloat:
		switch p := ptr.(type) {
		case *float64:
			return *p == 0
		case *float32:
			return *p == 0
		}
	case FieldBool:
		if p, ok := ptr.(*bool); ok {
			return !*p
		}
	case FieldBlob:
		if p, ok := ptr.(*[]byte); ok {
			return len(*p) == 0
		}
	case FieldIntSlice:
		if p, ok := ptr.(*[]int); ok {
			return len(*p) == 0
		}
	case FieldStructSlice:
		// Since we handle various slice types and tinywasm avoids reflection,
		// we check for nil pointer or rely on the specific implementation to provide length.
		// For zero-check purposes, if it's a pointer to a slice, we check if it's nil.
		return ptr == nil
	}
	return false
}

// ReadValues reads field values through Pointers by dereferencing based on FieldType.
// Used by consumers that need []any (e.g., orm for SQL args).
// Hot-path consumers (json, form) should read through Pointers directly to avoid boxing.
func ReadValues(schema []Field, ptrs []any) []any {
	vals := make([]any, len(schema))
	for i, f := range schema {
		if ptrs[i] == nil {
			continue
		}
		switch f.Type {
		case FieldText, FieldRaw:
			if p, ok := ptrs[i].(*string); ok && p != nil {
				vals[i] = *p
			}
		case FieldInt:
			switch p := ptrs[i].(type) {
			case *int64:
				if p != nil {
					vals[i] = *p
				}
			case *int:
				if p != nil {
					vals[i] = *p
				}
			case *int32:
				if p != nil {
					vals[i] = *p
				}
			case *uint:
				if p != nil {
					vals[i] = *p
				}
			case *uint32:
				if p != nil {
					vals[i] = *p
				}
			case *uint64:
				if p != nil {
					vals[i] = *p
				}
			}
		case FieldFloat:
			switch p := ptrs[i].(type) {
			case *float64:
				if p != nil {
					vals[i] = *p
				}
			case *float32:
				if p != nil {
					vals[i] = *p
				}
			}
		case FieldBool:
			if p, ok := ptrs[i].(*bool); ok && p != nil {
				vals[i] = *p
			}
		case FieldBlob:
			if p, ok := ptrs[i].(*[]byte); ok && p != nil {
				vals[i] = *p
			}
		case FieldStruct:
			vals[i] = ptrs[i] // pointer to nested struct IS the Fielder
		case FieldIntSlice:
			if p, ok := ptrs[i].(*[]int); ok && p != nil {
				vals[i] = *p
			}
		case FieldStructSlice:
			vals[i] = ptrs[i] // Pass the slice pointer as is
		}
	}
	return vals
}

// ReadStringPtr reads a string from a typed pointer.
// Returns the string value and true if the pointer is *string, or ("", false) otherwise.
func ReadStringPtr(ptr any) (string, bool) {
	if p, ok := ptr.(*string); ok && p != nil {
		return *p, true
	}
	return "", false
}

package model

import "github.com/tinywasm/fmt"

// Kind replaces the Field.Type enum slot + Field.Widget pair.
// Implementations are stateless templates, safe for concurrent reuse.
type Kind interface {
	Storage() FieldType          // deterministic Go/DDL mapping
	Name() string                // semantic name: "text", "int", "email", ...
	Validate(value string) error // ALWAYS present — fail-closed
}

type baseKind struct {
	storage FieldType
	name    string
	valid   func(value string) error
}

func (k baseKind) Storage() FieldType          { return k.storage }
func (k baseKind) Name() string                { return k.name }
func (k baseKind) Validate(value string) error { return k.valid(value) }

// Text returns the base Text kind.
// Its whitelist is the input-boundary XSS floor: excludes HTML-dangerous chars (<>"'&`).
func Text() Kind {
	return baseKind{
		storage: FieldText,
		name:    fieldTypeNames[FieldText],
		valid: func(value string) error {
			p := Permitted{
				Letters: true,
				Numbers: true,
				Spaces:  true,
				Tilde:   true,
				Extra:   []rune(".,;:-_@/()!?"),
			}
			// Kind validation doesn't have the field name context;
			// Field.Validate wraps/handles the error if needed.
			return p.Validate("", value)
		},
	}
}

// Int returns the base Int kind.
// Accepts digits and an optional leading minus sign.
func Int() Kind {
	return baseKind{
		storage: FieldInt,
		name:    fieldTypeNames[FieldInt],
		valid: func(value string) error {
			if value == "" {
				return nil
			}
			for i, r := range value {
				if i == 0 && r == '-' {
					if len(value) == 1 {
						return fmt.Err("int", "invalid")
					}
					continue
				}
				if r < '0' || r > '9' {
					return fmt.Err("int", "invalid")
				}
			}
			return nil
		},
	}
}

// Float returns the base Float kind.
// Accepts digits, at most one '.', and an optional leading minus sign.
func Float() Kind {
	return baseKind{
		storage: FieldFloat,
		name:    fieldTypeNames[FieldFloat],
		valid: func(value string) error {
			if value == "" {
				return nil
			}
			hasDot := false
			for i, r := range value {
				if i == 0 && r == '-' {
					if len(value) == 1 {
						return fmt.Err("float", "invalid")
					}
					continue
				}
				if r == '.' {
					if hasDot {
						return fmt.Err("float", "invalid")
					}
					hasDot = true
					continue
				}
				if r < '0' || r > '9' {
					return fmt.Err("float", "invalid")
				}
			}
			return nil
		},
	}
}

// Bool returns the base Bool kind.
// Accepts "true", "false", "1", "0", "".
func Bool() Kind {
	return baseKind{
		storage: FieldBool,
		name:    fieldTypeNames[FieldBool],
		valid: func(value string) error {
			switch value {
			case "true", "false", "1", "0", "":
				return nil
			default:
				return fmt.Err("bool", "invalid")
			}
		},
	}
}

// Blob returns the base Blob kind.
// No content validation (binary).
func Blob() Kind {
	return baseKind{
		storage: FieldBlob,
		name:    fieldTypeNames[FieldBlob],
		valid:   func(string) error { return nil },
	}
}

// Raw returns the base Raw kind.
// No content validation (pre-serialized JSON).
func Raw() Kind {
	return baseKind{
		storage: FieldRaw,
		name:    fieldTypeNames[FieldRaw],
		valid:   func(string) error { return nil },
	}
}

// Struct returns the base Struct kind.
// No string validation (nested Fielder validates itself).
func Struct() Kind {
	return baseKind{
		storage: FieldStruct,
		name:    fieldTypeNames[FieldStruct],
		valid:   func(string) error { return nil },
	}
}

// IntSlice returns the base IntSlice kind.
// No string validation.
func IntSlice() Kind {
	return baseKind{
		storage: FieldIntSlice,
		name:    fieldTypeNames[FieldIntSlice],
		valid:   func(string) error { return nil },
	}
}

// StructSlice returns the base StructSlice kind.
// No string validation.
func StructSlice() Kind {
	return baseKind{
		storage: FieldStructSlice,
		name:    fieldTypeNames[FieldStructSlice],
		valid:   func(string) error { return nil },
	}
}

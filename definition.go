package model

// Fields is the ordered list of field definitions of a model.
// Alias (not a named type) so a []Field literal is assignable without conversion.
type Fields = []Field

// Definition is the hand-written, fully-typed source of truth for a model.
// From a Definition, the ormc generator produces the concrete Go struct and its
// zero-reflection plumbing (Schema/Pointers/EncodeFields/DecodeFields/List).
//
// This inverts the previous flow (struct + string tags → generated schema):
// the schema literal is now authored by hand and everything else is derived.
type Definition struct {
	Name   string // model identity: table name, ModelName(), route key
	Fields Fields // ordered schema; widgets come from any model.Widget (e.g. form/input)
}

// Field returns the field with the given name and true, or a zero Field and false.
func (d Definition) Field(name string) (Field, bool) {
	for _, f := range d.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

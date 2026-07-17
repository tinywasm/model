package model

// FieldExt extends Field with foreign-key metadata for adapters that emit FK
// constraints (SQL compilers, DDL exporters). Moved here from tinywasm/ddlc:
// it describes data the model declares (via SchemaExt()), not a generation
type FieldExt struct {
	Field
	Ref       string // FK: target table name. Empty = no FK.
	RefColumn string // FK: target column. Empty = auto-detect PK of Ref table.
	OnDelete  string // Override ON DELETE action. Empty = CASCADE (default for all FKs).
}

package model_test

import (
	"testing"

	. "github.com/tinywasm/model"
)

func TestFieldTypeString(t *testing.T) {
	tests := []struct {
		ft   FieldType
		want string
	}{
		{FieldText, "text"},
		{FieldInt, "int"},
		{FieldFloat, "float"},
		{FieldBool, "bool"},
		{FieldBlob, "blob"},
		{FieldStruct, "struct"},
		{FieldIntSlice, "intslice"},
		{FieldStructSlice, "structslice"},
		{FieldRaw, "raw"},
		{FieldType(-1), "unknown"},
		{FieldType(9), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ft.String(); got != tt.want {
				t.Errorf("FieldType(%d).String() = %v, want %v", tt.ft, got, tt.want)
			}
		})
	}
}

// A03 Injection/XSS — Kind validation baseline
func TestBaseKinds(t *testing.T) {
	tests := []struct {
		kind    Kind
		storage FieldType
		name    string
		valid   string
		invalid string
	}{
		{Text(), FieldText, "text", "Hello World!", "<script>"},
		{Int(), FieldInt, "int", "-123", "12a"},
		{Float(), FieldFloat, "float", "-123.45", "1.2.3"},
		{Bool(), FieldBool, "bool", "true", "yes"},
		{Blob(), FieldBlob, "blob", "anything", ""}, // valid is just a dummy
		{Raw(), FieldRaw, "raw", "{}", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind.Storage() != tt.storage {
				t.Errorf("Storage() = %v, want %v", tt.kind.Storage(), tt.storage)
			}
			if tt.kind.Name() != tt.name {
				t.Errorf("Name() = %v, want %v", tt.kind.Name(), tt.name)
			}
			if err := tt.kind.Validate(tt.valid); err != nil {
				t.Errorf("Validate(%q) failed: %v", tt.valid, err)
			}
			if tt.invalid != "" {
				if err := tt.kind.Validate(tt.invalid); err == nil {
					t.Errorf("Validate(%q) should have failed", tt.invalid)
				}
			}
		})
	}
}

// A04 Insecure Design — fail-closed validation
func TestFieldValidate_FailClosed(t *testing.T) {
	// Text() kind must reject HTML-dangerous chars even without Permitted rules
	f := Field{
		Name: "x",
		Type: Text(),
	}
	if err := f.Validate("<script>"); err == nil {
		t.Error("expected Text() kind to reject <script>, but it passed")
	}

	// Nil kind must error
	f2 := Field{Name: "y", Type: nil}
	if err := f2.Validate("v"); err == nil {
		t.Error("expected error for nil kind, got nil")
	}
}

// Contract (settled 2026-07-10): the base Text kind's charset is a DEFAULT
// floor. A field declaring its own positive whitelist REPLACES it — the
// author's explicit charset governs and the author owns the output-encoding
// duty for any dangerous character it whitelists.
func TestFieldValidate_ExplicitWhitelistReplacesTextFloor(t *testing.T) {
	f := Field{
		Name:      "x",
		Type:      Text(),
		Permitted: Permitted{Extra: []rune{'<'}}, // author explicitly allows '<'
	}
	if err := f.Validate("<"); err != nil {
		t.Errorf("explicit field whitelist must replace the Text() floor, got %v", err)
	}
	// ...and the explicit whitelist still governs in the other direction:
	// anything NOT listed keeps failing ('a' is not in Extra{'<'}).
	if err := f.Validate("a"); err == nil {
		t.Error("chars outside the explicit field whitelist must fail")
	}
}

// The floor is only lifted for the BASE Text kind and only by a POSITIVE
// whitelist: restrictive-only rules (NotAllowed) and non-text kinds keep
// the kind validation intact.
func TestFieldValidate_FloorKeptWithoutPositiveWhitelist(t *testing.T) {
	// NotAllowed alone is not a whitelist — Text() floor still applies.
	f := Field{
		Name:      "x",
		Type:      Text(),
		Permitted: Permitted{NotAllowed: []string{"admin"}},
	}
	if err := f.Validate("<"); err == nil {
		t.Error("NotAllowed alone must not lift the Text() charset floor")
	}

	// Non-text kind: positive rules never skip the kind's format validation.
	n := Field{
		Name:      "n",
		Type:      Int(),
		Permitted: Permitted{Letters: true},
	}
	if err := n.Validate("abc"); err == nil {
		t.Error("Int() format validation must run regardless of field char rules")
	}
}

func TestFieldValidate_NotNull_EmptyValue(t *testing.T) {
	f := Field{
		Name:    "name",
		NotNull: true,
		Type:    Text(),
	}

	if err := f.Validate(""); err == nil {
		t.Error("expected error for NotNull, got nil")
	}
}

func TestFieldZeroValue(t *testing.T) {
	var f Field
	if f.Name != "" {
		t.Errorf("expected empty Name, got %v", f.Name)
	}
	if f.Type != nil {
		t.Errorf("expected nil Type, got %v", f.Type)
	}
	if f.IsPK() || f.IsUnique() || f.NotNull || f.IsAutoInc() {
		t.Errorf("expected all bools false, got PK=%v, Unique=%v, NotNull=%v, AutoInc=%v", f.IsPK(), f.IsUnique(), f.NotNull, f.IsAutoInc())
	}
}

func TestFieldOmitEmpty(t *testing.T) {
	f := Field{OmitEmpty: true}
	if !f.OmitEmpty {
		t.Error("expected OmitEmpty true")
	}
}

func TestFieldConstraints(t *testing.T) {
	f := Field{
		Name:    "id",
		Type:    Int(),
		NotNull: true,
		DB: &FieldDB{
			PK:      true,
			Unique:  true,
			AutoInc: true,
		},
	}
	if f.Name != "id" {
		t.Errorf("expected Name 'id', got %v", f.Name)
	}
	if f.Type.Storage() != FieldInt {
		t.Errorf("expected FieldInt storage, got %v", f.Type.Storage())
	}
	if !f.IsPK() {
		t.Error("expected PK true")
	}
	if !f.IsUnique() {
		t.Error("expected Unique true")
	}
	if !f.NotNull {
		t.Error("expected NotNull true")
	}
	if !f.IsAutoInc() {
		t.Error("expected AutoInc true")
	}
}

func TestFieldValidate(t *testing.T) {
	tests := []struct {
		name    string
		field   Field
		value   string
		wantErr bool
	}{
		{"NotNull empty", Field{NotNull: true, Type: Text()}, "", true},
		{"NotNull not empty", Field{NotNull: true, Type: Text()}, "foo", false},
		{"Nullable empty", Field{NotNull: false, Type: Text()}, "", false},
		{"With rules pass", Field{Type: Text(), Permitted: Permitted{Numbers: true}}, "123", false},
		{"With rules fail", Field{Type: Text(), Permitted: Permitted{Numbers: true}}, "abc", true},
		{"No rules pass", Field{Name: "any", Type: Text()}, "any value", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.field.Validate(tt.value); (err != nil) != tt.wantErr {
				t.Errorf("Field.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type fullMock struct {
	Text    string
	Int     int
	Int32   int32
	Int64   int64
	U       uint
	U32     uint32
	U64     uint64
	Float   float64
	Float32 float32
	Bool    bool
	Blob    []byte
	Raw     string
	Nested  *mockUser
}

func (m *fullMock) Schema() []Field {
	return []Field{
		{Name: "text", Type: Text(), NotNull: true},
		{Name: "int", Type: Int(), NotNull: true},
		{Name: "int32", Type: Int()},
		{Name: "int64", Type: Int()},
		{Name: "u", Type: Int()},
		{Name: "u32", Type: Int()},
		{Name: "u64", Type: Int()},
		{Name: "float", Type: Float(), NotNull: true},
		{Name: "float32", Type: Float()},
		{Name: "bool", Type: Bool(), NotNull: true},
		{Name: "blob", Type: Blob(), NotNull: true},
		{Name: "raw", Type: Raw(), NotNull: true},
		{Name: "nested", Type: Struct(&Definition{Name: "user"}), NotNull: true},
	}
}

func (m *fullMock) Pointers() []any {
	return []any{&m.Text, &m.Int, &m.Int32, &m.Int64, &m.U, &m.U32, &m.U64, &m.Float, &m.Float32, &m.Bool, &m.Blob, &m.Raw, m.Nested}
}

type mockUser struct {
	id   string
	name string
}

func (m *mockUser) Schema() []Field {
	return []Field{
		{Name: "id", Type: Text(), DB: &FieldDB{PK: true}},
		{Name: "name", Type: Text(), NotNull: true},
	}
}
func (m *mockUser) Pointers() []any            { return []any{&m.id, &m.name} }
func (m *mockUser) Validate(action byte) error { return ValidateFields(action, m) }

func TestActionConstantValues(t *testing.T) {
	if ActionCreate != 'c' || ActionRead != 'r' || ActionUpdate != 'u' || ActionDelete != 'd' {
		t.Fatal("action constants must keep their CRUD byte values — ecosystem wire/RBAC contract")
	}
}

func TestValidateFieldsRecursive(t *testing.T) {
	m := &fullMock{
		Text:   "hello",
		Int:    1,
		Float:  1.1,
		Bool:   true,
		Blob:   []byte{1},
		Raw:    "{}",
		Nested: &mockUser{id: "u1", name: "Alice"},
	}

	if err := ValidateFields(ActionUpdate, m); err != nil {
		t.Errorf("expected success, got %v", err)
	}

	// Fail nested
	m.Nested.name = ""
	if err := ValidateFields(ActionUpdate, m); err == nil {
		t.Error("expected failure in nested struct")
	}

	// Fail other types
	m.Nested.name = "Alice"
	m.Int = 0 // NotNull
	if err := ValidateFields(ActionUpdate, m); err == nil {
		t.Error("expected failure for int zero")
	}
}

func TestReadValuesAllTypes(t *testing.T) {
	m := &fullMock{
		Text:    "text",
		Int:     10,
		Int32:   32,
		Int64:   64,
		U:       100,
		U32:     320,
		U64:     640,
		Float:   1.1,
		Float32: 2.2,
		Bool:    true,
		Blob:    []byte{0x01},
		Raw:     "{\"a\":1}",
		Nested:  &mockUser{id: "u1", name: "Alice"},
	}
	schema := m.Schema()
	ptrs := m.Pointers()
	vals := ReadValues(schema, ptrs)

	expected := []any{"text", 10, int32(32), int64(64), uint(100), uint32(320), uint64(640), 1.1, float32(2.2), true, []byte{0x01}, "{\"a\":1}", m.Nested}
	for i, v := range expected {
		if i == 10 { // Blob check
			b1 := v.([]byte)
			b2 := vals[i].([]byte)
			if len(b1) != len(b2) || b1[0] != b2[0] {
				t.Errorf("mismatch at index %d: got %v, want %v", i, vals[i], v)
			}
			continue
		}
		if vals[i] != v {
			t.Errorf("mismatch at index %d: got %v, want %v", i, vals[i], v)
		}
	}

	// Test nil pointers
	ptrsCopy := make([]any, len(ptrs))
	copy(ptrsCopy, ptrs)
	ptrsCopy[0] = nil
	valsNil := ReadValues(schema, ptrsCopy)
	if valsNil[0] != nil {
		t.Error("expected nil for nil pointer")
	}
}

func TestIsZeroPtrAllTypes(t *testing.T) {
	var s string = ""
	if !IsZeroPtr(&s, FieldText) {
		t.Error("empty string should be zero")
	}
	if !IsZeroPtr(&s, FieldRaw) {
		t.Error("empty raw string should be zero")
	}
	s = "v"
	if IsZeroPtr(&s, FieldText) {
		t.Error("non-empty string should not be zero")
	}
	if IsZeroPtr(&s, FieldRaw) {
		t.Error("non-empty raw string should not be zero")
	}

	var i int = 0
	if !IsZeroPtr(&i, FieldInt) {
		t.Error("int 0 should be zero")
	}
	i = 1
	if IsZeroPtr(&i, FieldInt) {
		t.Error("int 1 should not be zero")
	}

	var i32 int32 = 0
	if !IsZeroPtr(&i32, FieldInt) {
		t.Error("int32 0 should be zero")
	}
	var i64 int64 = 0
	if !IsZeroPtr(&i64, FieldInt) {
		t.Error("int64 0 should be zero")
	}
	var u uint = 0
	if !IsZeroPtr(&u, FieldInt) {
		t.Error("uint 0 should be zero")
	}
	var u32 uint32 = 0
	if !IsZeroPtr(&u32, FieldInt) {
		t.Error("uint32 0 should be zero")
	}
	var u64 uint64 = 0
	if !IsZeroPtr(&u64, FieldInt) {
		t.Error("uint64 0 should be zero")
	}

	var f64 float64 = 0
	if !IsZeroPtr(&f64, FieldFloat) {
		t.Error("float64 0 should be zero")
	}
	f64 = 0.1
	if IsZeroPtr(&f64, FieldFloat) {
		t.Error("float64 0.1 should not be zero")
	}
	var f32 float32 = 0
	if !IsZeroPtr(&f32, FieldFloat) {
		t.Error("float32 0 should be zero")
	}

	var b bool = false
	if !IsZeroPtr(&b, FieldBool) {
		t.Error("bool false should be zero")
	}
	b = true
	if IsZeroPtr(&b, FieldBool) {
		t.Error("bool true should not be zero")
	}

	var bl []byte
	if !IsZeroPtr(&bl, FieldBlob) {
		t.Error("nil blob should be zero")
	}
	bl = []byte{}
	if !IsZeroPtr(&bl, FieldBlob) {
		t.Error("empty blob should be zero")
	}
	bl = []byte{1}
	if IsZeroPtr(&bl, FieldBlob) {
		t.Error("non-empty blob should not be zero")
	}

	var sl []int
	if !IsZeroPtr(&sl, FieldIntSlice) {
		t.Error("nil slice should be zero")
	}
	sl = []int{}
	if !IsZeroPtr(&sl, FieldIntSlice) {
		t.Error("empty slice should be zero")
	}
	sl = []int{1}
	if IsZeroPtr(&sl, FieldIntSlice) {
		t.Error("non-empty slice should not be zero")
	}

	var stl []Fielder
	if !IsZeroPtr(nil, FieldStructSlice) {
		t.Error("nil struct slice should be zero")
	}
	stl = []Fielder{&mockUser{}}
	if IsZeroPtr(&stl, FieldStructSlice) {
		t.Error("non-nil struct slice pointer should not be zero")
	}
}

func TestReadStringPtr(t *testing.T) {
	s := "hello"
	val, ok := ReadStringPtr(&s)
	if !ok || val != "hello" {
		t.Errorf("ReadStringPtr failed: got (%q, %v), want (\"hello\", true)", val, ok)
	}

	val, ok = ReadStringPtr("not a pointer")
	if ok {
		t.Error("ReadStringPtr should have failed for non-pointer")
	}

	var ns *string
	val, ok = ReadStringPtr(ns)
	if ok {
		t.Error("ReadStringPtr should have failed for nil pointer")
	}
}

type fielderOnlyMock struct {
	id string
}

func (m *fielderOnlyMock) Schema() []Field { return []Field{{Name: "id", Type: Text()}} }
func (m *fielderOnlyMock) Pointers() []any { return []any{&m.id} }

func TestValidateFieldsWithOnlyFielder(t *testing.T) {
	// Nested struct that only implements Fielder, not Validator.
	sub := &fielderOnlyMock{id: "ok"}
	schema := []Field{{Name: "sub", Type: Struct(&Definition{Name: "sub"})}}
	ptrs := []any{sub}

	// Validate it through the manualFielder helper
	if err := ValidateFields(ActionUpdate, &manualFielder{schema, ptrs}); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestValidateFieldsActions(t *testing.T) {
	type userActionMock struct {
		ID      int
		Name    string
		Email   string
		Raw     string
		Version int
	}

	schema := []Field{
		{Name: "id", Type: Int(), DB: &FieldDB{PK: true, AutoInc: true}},
		{Name: "name", Type: Text(), NotNull: true},
		{Name: "email", Type: Text(), Permitted: Permitted{Letters: true, Extra: []rune{'@', '.'}}},
		{Name: "raw", Type: Raw(), NotNull: true},
		{Name: "version", Type: Int(), NotNull: true},
	}

	// Helper to create manual Fielder
	getFielder := func(m *userActionMock) Fielder {
		return &manualFielder{
			schema: schema,
			ptrs:   []any{&m.ID, &m.Name, &m.Email, &m.Raw, &m.Version},
		}
	}

	t.Run("Create 'c'", func(t *testing.T) {
		m := &userActionMock{Name: "Alice", Email: "a@b.com", Raw: "{}", Version: 1}
		f := getFielder(m)

		// PK+AutoInc should be skipped in 'c'
		if err := ValidateFields(ActionCreate, f); err != nil {
			t.Errorf("expected success in ActionCreate with zero ID, got %v", err)
		}

		// NotNull still applies
		m.Name = ""
		if err := ValidateFields(ActionCreate, f); err == nil {
			t.Error("expected failure in ActionCreate with empty Name")
		}
		m.Name = "Alice"
		m.Raw = ""
		if err := ValidateFields(ActionCreate, f); err == nil {
			t.Error("expected failure in ActionCreate with empty Raw")
		}
	})

	t.Run("Update 'u'", func(t *testing.T) {
		m := &userActionMock{ID: 1, Name: "Alice", Email: "a@b.com", Raw: "{}", Version: 1}
		f := getFielder(m)

		if err := ValidateFields(ActionUpdate, f); err != nil {
			t.Errorf("expected success in ActionUpdate, got %v", err)
		}

		// PK is required in 'u'
		m.ID = 0
		if err := ValidateFields(ActionUpdate, f); err == nil {
			t.Error("expected failure in ActionUpdate with zero ID")
		}
	})

	t.Run("Delete 'd'", func(t *testing.T) {
		m := &userActionMock{ID: 1, Name: "", Email: "invalid!!", Version: 0}
		f := getFielder(m)

		// Only PK matters in 'd'
		if err := ValidateFields(ActionDelete, f); err != nil {
			t.Errorf("expected success in ActionDelete with only PK, got %v", err)
		}

		// PK missing
		m.ID = 0
		if err := ValidateFields(ActionDelete, f); err == nil {
			t.Error("expected failure in ActionDelete with missing PK")
		}
	})

	t.Run("Unknown 'x'", func(t *testing.T) {
		m := &userActionMock{ID: 1, Name: "Alice", Email: "a@b.com", Raw: "{}", Version: 1}
		f := getFielder(m)

		// Should behave like 'u'
		if err := ValidateFields('x', f); err != nil {
			t.Errorf("expected success in 'x', got %v", err)
		}

		m.ID = 0
		if err := ValidateFields('x', f); err == nil {
			t.Error("expected failure in 'x' with missing PK")
		}
	})
}

type manualFielder struct {
	schema []Field
	ptrs   []any
}

func (f *manualFielder) Schema() []Field { return f.schema }
func (f *manualFielder) Pointers() []any { return f.ptrs }

type mockFielderSlice struct {
	items []Fielder
}

func (m *mockFielderSlice) Schema() []Field  { return nil }
func (m *mockFielderSlice) Pointers() []any  { return nil }
func (m *mockFielderSlice) Len() int         { return len(m.items) }
func (m *mockFielderSlice) At(i int) Fielder { return m.items[i] }
func (m *mockFielderSlice) Append() Fielder  { return nil }

func TestFielderSliceEmbedsFielder(t *testing.T) {
	var _ Fielder = (FielderSlice)(nil)
	var _ Fielder = (*mockFielderSlice)(nil)
}

func TestRefKind(t *testing.T) {
	def := &Definition{Name: "user"}
	kinds := []struct {
		name string
		kind Kind
	}{
		{"struct", Struct(def)},
		{"structslice", StructSlice(def)},
	}

	for _, tt := range kinds {
		t.Run(tt.name, func(t *testing.T) {
			rk, ok := tt.kind.(RefKind)
			if !ok {
				t.Fatalf("%s kind does not implement RefKind", tt.name)
			}
			if rk.Ref() != def {
				t.Errorf("Ref() = %v, want %v", rk.Ref(), def)
			}
			if err := tt.kind.Validate("any"); err != nil {
				t.Errorf("Validate() failed: %v", err)
			}
		})
	}
}

func TestNilRefBackstop(t *testing.T) {
	kinds := []Kind{Struct(nil), StructSlice(nil)}
	for _, k := range kinds {
		t.Run(k.Name(), func(t *testing.T) {
			err := k.Validate("x")
			if err == nil {
				t.Error("expected error for nil ref, got nil")
			}
			f := Field{Name: "f", Type: k}
			if err := f.Validate("x"); err == nil {
				t.Error("expected Field.Validate to fail for nil ref kind")
			}
		})
	}
}

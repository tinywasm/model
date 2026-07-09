package model_test

import (
	"testing"

	"github.com/tinywasm/model"
)

func TestDefinition(t *testing.T) {
	// Construct a Definition literal
	var UserModel = model.Definition{
		Name: "user",
		Fields: model.Fields{
			{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
			{Name: "name", Type: model.Text(), NotNull: true, Permitted: model.Permitted{Minimum: 2}},
			{Name: "email", Type: model.Text(), NotNull: true},
		},
	}

	// Assert Name and number of fields
	if UserModel.Name != "user" {
		t.Errorf("expected Name 'user', got %q", UserModel.Name)
	}
	if len(UserModel.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(UserModel.Fields))
	}

	// Definition.Field("email") returns the correct field
	f, ok := UserModel.Field("email")
	if !ok {
		t.Error("expected field 'email' to be found")
	}
	if f.Name != "email" {
		t.Errorf("expected field name 'email', got %q", f.Name)
	}

	// Definition.Field("nope") returns false
	_, ok = UserModel.Field("nope")
	if ok {
		t.Error("expected field 'nope' not to be found")
	}

	// Assert that Fields is assignable from a []Field literal without conversion (alias)
	var fields model.Fields = []model.Field{
		{Name: "test", Type: model.Text()},
	}
	_ = fields

	// Assert that Field{Type: FieldStruct, Ref: &SomeModel} preserves the Ref pointer
	SomeModel := &model.Definition{Name: "some"}
	fStruct := model.Field{Name: "nested", Type: model.Struct(), Ref: SomeModel}
	if fStruct.Ref != SomeModel {
		t.Error("Ref pointer not preserved")
	}

	// Assert that the zero-value of Ref is nil
	var fZero model.Field
	if fZero.Ref != nil {
		t.Error("expected zero-value Ref to be nil")
	}

	// Assert that Exclude defaults to false and can be set explicitly.
	if fZero.Exclude {
		t.Error("expected zero-value Exclude to be false")
	}
	fExcluded := model.Field{Name: "password_hash", Type: model.Text(), Exclude: true}
	if !fExcluded.Exclude {
		t.Error("expected Exclude to be true when set")
	}

	// Assert Ref disambiguated by Type: scalar FK usage (Ref + FieldDB.RefColumn/OnDelete).
	StaffModel := &model.Definition{Name: "staff"}
	fFK := model.Field{
		Name: "staff_id", Type: model.Int(), NotNull: true, Ref: StaffModel,
		DB: &model.FieldDB{RefColumn: "id", OnDelete: "CASCADE"},
	}
	if fFK.Ref != StaffModel {
		t.Error("expected scalar FK Ref to be preserved")
	}
	if fFK.DB.RefColumn != "id" || fFK.DB.OnDelete != "CASCADE" {
		t.Error("expected FieldDB.RefColumn/OnDelete to be preserved")
	}

	// Zero-value of the new FieldDB members must be empty strings.
	var dbZero model.FieldDB
	if dbZero.RefColumn != "" || dbZero.OnDelete != "" {
		t.Error("expected zero-value RefColumn/OnDelete to be empty")
	}
}

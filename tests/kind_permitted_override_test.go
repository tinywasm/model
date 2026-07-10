package model_test

import (
	"testing"

	. "github.com/tinywasm/model"
)

// Bug proof (2026-07-10, devbrowser TestRobustInteraction/TestBrowserSwipe):
// a Definition author declares Type: Text() and explicitly permits extra
// characters on the Field's embedded Permitted (e.g. '#' for CSS selectors).
// Expected (author intent): the explicit field-level whitelist governs.
// Actual: Field.Validate runs the Kind's charset floor FIRST and
// unconditionally, so Text() rejects "#btn" before the field's Permitted is
// even consulted — field-level rules can only tighten, never extend.
func TestFieldPermitted_ExtendsTextKindCharset(t *testing.T) {
	selector := Field{
		Name:    "selector",
		Type:    Text(),
		NotNull: true,
		Permitted: Permitted{
			Letters: true,
			Numbers: true,
			Spaces:  true,
			Extra:   []rune("#.-_[]='\"()>~+*:,"), // CSS selector charset
		},
	}

	cases := []string{
		"#btn",
		".card > a[href='#top']",
		"div.item:nth-child(2)",
	}
	for _, value := range cases {
		if err := selector.Validate(value); err != nil {
			t.Errorf("author explicitly permitted these chars on the Field, but Validate(%q) = %v", value, err)
		}
	}
}

// Control: the same field-level Permitted still rejects what the author did
// NOT include (the whitelist governs in both directions).
func TestFieldPermitted_StillRejectsUnlisted(t *testing.T) {
	selector := Field{
		Name: "selector",
		Type: Text(),
		Permitted: Permitted{
			Letters: true,
			Numbers: true,
			Extra:   []rune("#"),
		},
	}
	if err := selector.Validate("<script>"); err == nil {
		t.Error("chars outside the field whitelist must keep failing")
	}
}

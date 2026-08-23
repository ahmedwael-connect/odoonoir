package odoomod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	for _, ok := range []string{"sale_ext", "a1", "my_module"} {
		if err := ValidName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"Sale_Ext", "1sale", "sale.ext", "sale-ext"} {
		if err := ValidName(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestScaffold(t *testing.T) {
	dir := t.TempDir()
	m := &Module{
		Name:        "library_book",
		DisplayName: "Library Book",
		Summary:     "Manage books",
		Author:      "Tester",
		License:     "LGPL-3",
		Version:     "18.0.1.0.0",
		Category:    "Library",
		Depends:     []string{"base", "mail"},
		Menus:       true,
		Wizard:      true,
		Tests:       true,
		Models: []Model{{
			Name:  "library.book",
			Label: "Book",
			Fields: []Field{
				{Name: "name", Label: "Name", Type: "char", Required: true},
				{Name: "isbn", Label: "ISBN", Type: "char"},
				{Name: "publisher", Label: "Publisher", Type: "many2one", Relation: "res.partner"},
			},
		}},
		AddonsDir: dir,
	}
	if err := Scaffold(m); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "library_book")
	for _, f := range []string{
		"__manifest__.py", "__init__.py",
		"models/library_book.py", "models/library_book_wizard.py",
		"views/library_book_views.xml", "views/menus.xml", "views/wizard_views.xml",
		"security/ir.model.access.csv", "tests/test_library_book.py",
	} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	manifest, _ := os.ReadFile(filepath.Join(root, "__manifest__.py"))
	ms := string(manifest)
	for _, want := range []string{"'name': \"Library Book\"", "'version': \"18.0.1.0.0\"",
		"'license': \"LGPL-3\"", "views/library_book_views.xml", "'mail'", "'base'"} {
		if !strings.Contains(ms, want) {
			t.Errorf("manifest missing %q", want)
		}
	}
	if strings.Count(ms, "'data': [") != 1 {
		t.Errorf("manifest data block duplicated:\n%s", ms)
	}

	model, _ := os.ReadFile(filepath.Join(root, "models", "library_book.py"))
	ms = string(model)
	for _, want := range []string{"class LibraryBook(models.Model)", "_name = 'library.book'",
		"publisher = fields.Many2one('res.partner', string='Publisher')", "required=True"} {
		if !strings.Contains(ms, want) {
			t.Errorf("model missing %q", want)
		}
	}

	access, _ := os.ReadFile(filepath.Join(root, "security", "ir.model.access.csv"))
	if strings.Contains(string(access), "library.book,") {
		t.Errorf("access csv must use underscore ids:\n%s", access)
	}
}

func TestClassName(t *testing.T) {
	cases := map[string]string{
		"library.book":   "LibraryBook",
		"library_book.x": "LibraryBookX",
		"sale.order":     "SaleOrder",
	}
	for in, want := range cases {
		if got := ClassName(in); got != want {
			t.Errorf("ClassName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoModelsModuleFails(t *testing.T) {
	m := &Module{Name: "empty_mod", AddonsDir: t.TempDir()}
	if err := Scaffold(m); err != nil {
		t.Fatalf("scaffold without models should not fail at scaffold level: %v", err)
	}
}

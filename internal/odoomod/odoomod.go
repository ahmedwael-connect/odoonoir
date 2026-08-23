package odoomod

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Field describes a model field to scaffold.
type Field struct {
	Name     string
	Label    string
	Type     string // char, text, boolean, integer, float, date, datetime, selection, many2one, one2many, many2many
	Relation string // model for relational types
	Required bool
}

// Model describes a model to scaffold.
type Model struct {
	Name        string // technical, e.g. "library.book"
	Label       string
	Description string
	Fields      []Field
}

// Module describes the full scaffold request.
type Module struct {
	Name        string
	DisplayName string
	Summary     string
	Author      string
	License     string // LGPL-3, MIT, AGPL-3, OPL-1
	Version     string // module version, e.g. "17.0.1.0.0"
	Category    string
	Depends     []string
	Models      []Model
	Menus       bool
	Wizard      bool
	Tests       bool
	AddonsDir   string // target directory (instance custom_addons)
}

var typeDefaults = map[string]string{
	"char":      "fields.Char",
	"text":      "fields.Text",
	"html":      "fields.Html",
	"boolean":   "fields.Boolean",
	"integer":   "fields.Integer",
	"float":     "fields.Float",
	"monetary":  "fields.Monetary",
	"date":      "fields.Date",
	"datetime":  "fields.Datetime",
	"selection": "fields.Selection([('option1', 'Option 1')])",
	"many2one":  "fields.Many2one",
	"one2many":  "fields.One2many",
	"many2many": "fields.Many2many",
}

// ValidName validates a module technical name.
func ValidName(name string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(name) {
		return fmt.Errorf("invalid module name %q: lowercase letters, digits, underscores, starting with a letter", name)
	}
	return nil
}

// ClassName converts a model technical name to an Odoo class name.
func ClassName(model string) string {
	parts := strings.Split(model, ".")
	for i, p := range parts {
		parts[i] = ModuleClassName(p)
	}
	return strings.Join(parts, "")
}

// ModuleClassName converts a module name to a CamelCase class name.
func ModuleClassName(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// Scaffold writes the module tree to AddonsDir/<Name>.
func Scaffold(m *Module) error {
	if err := ValidName(m.Name); err != nil {
		return err
	}
	root := filepath.Join(m.AddonsDir, m.Name)
	dirs := []string{root, filepath.Join(root, "models"), filepath.Join(root, "views"),
		filepath.Join(root, "security"), filepath.Join(root, "data"), filepath.Join(root, "tests")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	files := map[string]string{
		"__manifest__.py":              m.manifest(),
		"__init__.py":                  m.pkgInit(),
		"models/__init__.py":           m.modelsInit(),
		"security/ir.model.access.csv": m.accessCSV(),
		"data/ir.model.access.csv":     "", // placeholder, removed below if unused
	}
	for _, mod := range m.Models {
		files["models/"+mod.FileName()+".py"] = m.modelFile(mod)
	}
	if m.Menus {
		files["views/menus.xml"] = m.menusXML()
	}
	for _, mod := range m.Models {
		files["views/"+mod.FileName()+"_views.xml"] = m.viewsXML(mod)
	}
	if m.Wizard {
		files["models/"+m.wizardName()+"_wizard.py"] = m.wizardFile()
		files["views/wizard_views.xml"] = m.wizardViewsXML()
	}
	if m.Tests {
		files["tests/__init__.py"] = "from . import test_" + m.Name + "\n"
		files["tests/test_"+m.Name+".py"] = m.testFile()
	}
	delete(files, "data/ir.model.access.csv")

	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) manifest() string {
	depends := strings.Join(m.Depends, "',\n        '")
	data := []string{}
	if m.Menus {
		data = append(data, "        'views/menus.xml',")
	}
	for _, mod := range m.Models {
		data = append(data, fmt.Sprintf("        'views/%s_views.xml',", mod.FileName()))
	}
	if m.Wizard {
		data = append(data, "        'views/wizard_views.xml',")
	}
	dataStr := ""
	if len(data) > 0 {
		dataStr = strings.Join(data, "\n") + "\n"
	}
	return fmt.Sprintf(`# -*- coding: utf-8 -*-
{
    'name': %q,
    'summary': %q,
    'version': %q,
    'category': %q,
    'author': %q,
    'license': %q,
    'depends': [
        '%s'
    ],
%s    'demo': [],
    'installable': True,
    'application': True,
    'auto_install': False,
}
`, m.DisplayName, m.Summary, m.Version, m.Category, m.Author, m.License, depends, dataBlock(dataStr))
}

func dataBlock(data string) string {
	if data == "" {
		return ""
	}
	return "    'data': [\n" + data + "    ],\n"
}

func (m *Module) pkgInit() string {
	var b strings.Builder
	b.WriteString("# -*- coding: utf-8 -*-\n")
	for _, mod := range m.Models {
		b.WriteString("from . import " + mod.FileName() + "\n")
	}
	if m.Wizard {
		b.WriteString("from . import " + m.wizardName() + "_wizard\n")
	}
	return b.String()
}

func (m *Module) modelsInit() string {
	var b strings.Builder
	b.WriteString("# -*- coding: utf-8 -*-\n")
	for _, mod := range m.Models {
		b.WriteString("from . import " + mod.FileName() + "\n")
	}
	return b.String()
}

func (m *Module) modelFile(mod Model) string {
	var fields []string
	for _, f := range mod.Fields {
		fdef := typeDefaults[f.Type]
		if f.Type == "many2one" || f.Type == "one2many" || f.Type == "many2many" {
			rel := f.Relation
			if rel == "" {
				rel = "res.partner"
			}
			fdef = fdef + "('" + rel + "'"
			if f.Type == "many2one" {
				fdef += ", string='" + f.Label + "'"
			}
			fdef += ")"
		} else if f.Type == "selection" {
			// keep placeholder
		} else {
			fdef = fdef + "(string='" + f.Label + "'"
			if f.Required {
				fdef += ", required=True"
			}
			fdef += ")"
		}
		fields = append(fields, fmt.Sprintf("    %s = %s", f.Name, fdef))
	}
	description := mod.Description
	if description == "" {
		description = mod.Label
	}
	return fmt.Sprintf(`# -*- coding: utf-8 -*-
from odoo import fields, models


class %s(models.Model):
    """%s."""

    _name = '%s'
    _description = %q
    _rec_name = 'name'

%s

    def name_get(self):
        """Overridden to customize display name."""
        return super().name_get()
`, ClassName(mod.Name), description, mod.Name, mod.Label, defaultFields(fields))
}

func defaultFields(fields []string) string {
	if len(fields) > 0 {
		return strings.Join(fields, "\n")
	}
	return "    name = fields.Char(string='Name', required=True)"
}

func (m *Module) accessCSV() string {
	var b strings.Builder
	b.WriteString("id,name,model_id:id,group_id:id,perm_read,perm_write,perm_create,perm_unlink\n")
	for _, mod := range m.Models {
		fn := mod.FileName()
		b.WriteString(fmt.Sprintf(
			"access_%s_user,access_%s_user,model_%s,base.group_user,1,1,1,1\n",
			fn, fn, fn))
	}
	return b.String()
}

func (m *Module) viewsXML(mod Model) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<odoo>
    <record id="view_` + m.Name + `_` + mod.FileName() + `_list" model="ir.ui.view">
        <field name="name">` + m.DisplayName + ` list</field>
        <field name="model">` + mod.Name + `</field>
        <field name="arch" type="xml">
            <tree string="` + mod.Label + `">
`)
	for _, f := range mod.Fields {
		if f.Type == "text" || f.Type == "one2many" || f.Type == "many2many" || f.Type == "html" {
			continue
		}
		b.WriteString("                <field name=\"" + f.Name + "\"/>\n")
	}
	b.WriteString(`            </tree>
        </field>
    </record>
    <record id="view_` + m.Name + `_` + mod.FileName() + `_form" model="ir.ui.view">
        <field name="name">` + m.DisplayName + ` form</field>
        <field name="model">` + mod.Name + `</field>
        <field name="arch" type="xml">
            <form string="` + mod.Label + `">
                <sheet>
                    <group>
`)
	for _, f := range mod.Fields {
		b.WriteString("                        <field name=\"" + f.Name + "\"/>\n")
	}
	b.WriteString(`                    </group>
                </sheet>
            </form>
        </field>
    </record>
    <record id="action_` + m.Name + `_` + mod.FileName() + `" model="ir.actions.act_window">
        <field name="name">` + mod.Label + `</field>
        <field name="res_model">` + mod.Name + `</field>
        <field name="view_mode">tree,form</field>
    </record>
</odoo>
`)
	return b.String()
}

func (m *Module) menusXML() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<odoo>
    <menuitem id="menu_` + m.Name + `_root" name="` + m.DisplayName + `"/>
`)
	for i, mod := range m.Models {
		b.WriteString(fmt.Sprintf(`    <menuitem id="menu_%[1]s_%[2]s" parent="menu_%[1]s_root" name="%[3]s" action="action_%[1]s_%[2]s" sequence="%[4]d"/>
`, m.Name, mod.FileName(), mod.Label, i+10))
	}
	b.WriteString("</odoo>\n")
	return b.String()
}

func (m *Module) wizardName() string { return m.Name }

func (m *Module) wizardFile() string {
	return fmt.Sprintf(`# -*- coding: utf-8 -*-
from odoo import fields, models


class %sWizard(models.TransientModel):
    """%s wizard."""

    _name = '%s.wizard'
    _description = 'Wizard'

    note = fields.Text(string='Note', required=True)

    def action_apply(self):
        return {'type': 'ir.actions.act_window_close'}
`, ModuleClassName(m.Name), m.DisplayName, m.Name)
}

func (m *Module) wizardViewsXML() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<odoo>
    <record id="view_%s_wizard" model="ir.ui.view">
        <field name="name">%s wizard form</field>
        <field name="model">%s.wizard</field>
        <field name="arch" type="xml">
            <form string="%s">
                <group>
                    <field name="note"/>
                </group>
                <footer>
                    <button name="action_apply" string="Apply" type="object" class="btn-primary"/>
                    <button string="Cancel" class="btn-secondary" special="cancel"/>
                </footer>
            </form>
        </field>
    </record>
</odoo>
`, m.Name, m.DisplayName, m.Name, m.DisplayName)
}

func (m *Module) testFile() string {
	mod := m.Models[0]
	tests := "        record = self.env['" + mod.Name + "'].create({'name': 'Test'})\n        self.assertTrue(record)\n"
	return fmt.Sprintf(`# -*- coding: utf-8 -*-
from odoo.tests import TransactionCase


class Test%s(TransactionCase):

    def test_create(self):
        """Creating a record should succeed."""
%s
`, ModuleClassName(m.Name), tests)
}

// FileName converts a model technical name to a snake filename.
func (m Model) FileName() string { return strings.ReplaceAll(m.Name, ".", "_") }

// SortModels orders models for deterministic output.
func SortModels(models []Model) {
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
}

// TimestampedVersion builds a version string like 17.0.1.0.0.
func TimestampedVersion(major string) string {
	return major + ".0." + time.Now().Format("1.0.20060102")
}

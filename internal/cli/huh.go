package cli

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	"github.com/ahmed/odoonoir/internal/instance"
)

// huhCompatKeyMap returns a keymap that also accepts ctrl+j ("\n") for
// submit/next. Bubbletea maps a raw "\n" byte to ctrl+j (KeyCtrlJ), not
// enter. Keys typed in the brief window before a form switches the terminal
// to raw mode arrive as "\n" (ICRNL converts "\r"), and are otherwise
// silently ignored — which made sequential forms feel unresponsive.
func huhCompatKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	withCR := func(b key.Binding) key.Binding {
		return key.NewBinding(key.WithKeys(append(b.Keys(), "ctrl+j")...))
	}
	km.Confirm.Next = withCR(km.Confirm.Next)
	km.Confirm.Submit = withCR(km.Confirm.Submit)
	km.Input.Next = withCR(km.Input.Next)
	km.Input.Submit = withCR(km.Input.Submit)
	km.MultiSelect.Next = withCR(km.MultiSelect.Next)
	km.MultiSelect.Submit = withCR(km.MultiSelect.Submit)
	km.Note.Next = withCR(km.Note.Next)
	km.Note.Submit = withCR(km.Note.Submit)
	return km
}

// pickInstance asks the user to choose an instance interactively.
func pickInstance(title string) (*instance.Instance, error) {
	instances, err := reg.All()
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, errors.New("no instances yet — create one with: odoonoir create <name> -v 18")
	}
	opts := make([]huh.Option[string], 0, len(instances))
	byName := map[string]*instance.Instance{}
	for _, inst := range instances {
		label := fmt.Sprintf("%s   %s   port %d", inst.Name, inst.Version, inst.Port)
		if inst.Description != "" {
			label += "   " + inst.Description
		}
		opts = append(opts, huh.NewOption(label, inst.Name))
		byName[inst.Name] = inst
	}
	var chosen string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(title).
			Options(opts...).
			Value(&chosen),
	)).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithKeyMap(huhCompatKeyMap())
	if err := form.Run(); err != nil {
		return nil, err
	}
	return byName[chosen], nil
}

// pickInstanceName is like pickInstance but returns only the chosen name.
func pickInstanceName(title string) (string, error) {
	inst, err := pickInstance(title)
	if err != nil {
		return "", err
	}
	return inst.Name, nil
}

// pickDatabase asks which database of the instance to use.
func pickDatabase(inst *instance.Instance, title string) (string, error) {
	dbs := inst.AllDBs()
	if len(dbs) <= 1 {
		return inst.DBName, nil
	}
	opts := make([]huh.Option[string], 0, len(dbs))
	for _, d := range dbs {
		label := d
		if d == inst.DBName {
			label += "  (primary)"
		}
		opts = append(opts, huh.NewOption(label, d))
	}
	var chosen string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(title).
			Options(opts...).
			Value(&chosen),
	)).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithKeyMap(huhCompatKeyMap())
	if err := form.Run(); err != nil {
		return "", err
	}
	return chosen, nil
}

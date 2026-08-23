package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Ask prompts for free-form input with an optional default.
func Ask(question, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", question, def)
	} else {
		fmt.Printf("%s: ", question)
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// AskInt prompts for an integer with a default.
func AskInt(question string, def int) (int, error) {
	s, err := Ask(question, strconv.Itoa(def))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(s))
}

// Confirm asks a yes/no question, defaulting to def.
func Confirm(question string, def bool) (bool, error) {
	suffix := "[y/N]"
	if def {
		suffix = "[Y/n]"
	}
	ans, err := Ask(question+" "+suffix, "")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return def, nil
}

// MultiSelect lets the user pick multiple options by index.
// Empty input returns the defaults.
func MultiSelect(question string, options []string, defaults []string) ([]string, error) {
	fmt.Printf("%s\n", question)
	for i, opt := range options {
		mark := "  "
		if contains(defaults, opt) {
			mark = "* "
		}
		fmt.Printf("  [%2d] %s%s\n", i+1, mark, opt)
	}
	fmt.Printf("Enter numbers separated by commas (empty keeps * defaults): ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaults, nil
	}
	selected := map[string]bool{}
	for _, tok := range strings.Split(line, ",") {
		idx, err := strconv.Atoi(strings.TrimSpace(tok))
		if err != nil || idx < 1 || idx > len(options) {
			fmt.Printf("ignoring invalid choice %q\n", tok)
			continue
		}
		selected[options[idx-1]] = true
	}
	out := []string{}
	for _, opt := range options {
		if selected[opt] {
			out = append(out, opt)
		}
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

package repository

import (
	"strings"

	"github.com/manifoldco/promptui"
)

// TerminalPrompter implements usecase.Prompter using promptui.
type TerminalPrompter struct{}

func (p *TerminalPrompter) Confirm(label string) (bool, error) {
	prompt := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil {
		return false, err
	}
	return strings.ToLower(result) == "y", nil
}

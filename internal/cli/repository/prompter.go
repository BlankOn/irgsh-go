package repository

import (
	"errors"
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
		// promptui returns ErrAbort when the user types "n" or presses Enter
		if errors.Is(err, promptui.ErrAbort) {
			return false, nil
		}
		return false, err
	}
	return strings.ToLower(result) == "y", nil
}

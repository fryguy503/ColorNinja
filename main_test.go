package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestConfirmOverwriteAcceptsWindowsYes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.png")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	var dialog runtime.MessageDialogOptions
	overwrite, err := confirmOverwrite(path, func(options runtime.MessageDialogOptions) (string, error) {
		dialog = options
		// Wails 2.15's Windows QuestionDialog uses MB_YESNO and returns "Yes".
		return "Yes", nil
	})
	if err != nil || !overwrite {
		t.Fatalf("Windows approval rejected: overwrite=%v, error=%v", overwrite, err)
	}
	if dialog.Type != runtime.QuestionDialog || dialog.DefaultButton != "No" || dialog.CancelButton != "No" || len(dialog.Buttons) != 2 || dialog.Buttons[0] != "Yes" || dialog.Buttons[1] != "No" {
		t.Fatalf("dialog must use native Yes/No and default to No: %+v", dialog)
	}
}

func TestConfirmOverwriteDeclineAndDialogErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.png")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []string{"No", "Cancel", "", "Error", "Replace"} {
		t.Run(choice, func(t *testing.T) {
			overwrite, err := confirmOverwrite(path, func(runtime.MessageDialogOptions) (string, error) { return choice, nil })
			if err == nil || overwrite {
				t.Fatal("unapproved overwrite allowed", choice)
			}
			b, err := os.ReadFile(path)
			if err != nil || string(b) != "original" {
				t.Fatal("existing file changed", err)
			}
		})
	}
	dialogErr := errors.New("dialog unavailable")
	if overwrite, err := confirmOverwrite(path, func(runtime.MessageDialogOptions) (string, error) { return "Yes", dialogErr }); overwrite || !errors.Is(err, dialogErr) {
		t.Fatal("dialog failure was ignored", overwrite, err)
	}
}

func TestConfirmOverwriteNewFileDoesNotPrompt(t *testing.T) {
	overwrite, err := confirmOverwrite(filepath.Join(t.TempDir(), "new.png"), func(runtime.MessageDialogOptions) (string, error) {
		t.Fatal("prompted for a new file")
		return "", nil
	})
	if overwrite || err != nil {
		t.Fatal("new files must still use no-clobber publication", overwrite, err)
	}
}

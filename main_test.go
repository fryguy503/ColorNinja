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
	overwrite, err := confirmOverwrite(path, "", func(options runtime.MessageDialogOptions) (string, error) {
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
			overwrite, err := confirmOverwrite(path, "", func(runtime.MessageDialogOptions) (string, error) { return choice, nil })
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
	if overwrite, err := confirmOverwrite(path, "", func(runtime.MessageDialogOptions) (string, error) { return "Yes", dialogErr }); overwrite || !errors.Is(err, dialogErr) {
		t.Fatal("dialog failure was ignored", overwrite, err)
	}
}

func TestConfirmOverwriteNewFileDoesNotPrompt(t *testing.T) {
	overwrite, err := confirmOverwrite(filepath.Join(t.TempDir(), "new.png"), "", func(runtime.MessageDialogOptions) (string, error) {
		t.Fatal("prompted for a new file")
		return "", nil
	})
	if overwrite || err != nil {
		t.Fatal("new files must still use no-clobber publication", overwrite, err)
	}
}

func TestConfirmOverwriteDoesNotRepeatNativeSaveConfirmation(t *testing.T) {
	for _, name := range []string{"image.png", "palette.json", "layers.png", "project.hfp", "project.colorninja", "settings.colorninja-profile.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			overwrite, err := confirmOverwrite(path, path, func(runtime.MessageDialogOptions) (string, error) {
				t.Fatal("prompted again for the file approved by the native save dialog")
				return "", nil
			})
			if err != nil || !overwrite {
				t.Fatalf("native approval rejected: overwrite=%v, error=%v", overwrite, err)
			}
		})
	}
}

func TestConfirmOverwriteNativeSaveNewFileKeepsNoClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.hfp")
	overwrite, err := confirmOverwrite(path, path, func(runtime.MessageDialogOptions) (string, error) {
		t.Fatal("prompted for a new save path")
		return "", nil
	})
	if err != nil || overwrite {
		t.Fatalf("new save paths must still use no-clobber publication: overwrite=%v, error=%v", overwrite, err)
	}
}

func TestConfirmOverwriteNativeApprovalDoesNotCoverOtherPaths(t *testing.T) {
	for _, tc := range []struct {
		name, selected, output string
	}{
		{"added export extension", "image", "image.hfp"},
		{"different export extension", "image.png", "image.png.hfp"},
		{"added project extension", "project", "project.colorninja"},
		{"added profile extension", "settings", "settings.colorninja-profile.json"},
		{"project companion", "image.hfp", "image.hfp.colorninja"},
		{"profile companion", "image.hfp", "image.hfp.colorninja-profile.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			selected, output := filepath.Join(dir, tc.selected), filepath.Join(dir, tc.output)
			if err := os.WriteFile(output, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, choice := range []string{"Yes", "No"} {
				calls := 0
				overwrite, err := confirmOverwrite(output, selected, func(options runtime.MessageDialogOptions) (string, error) {
					calls++
					if options.Message != tc.output+" already exists. Replace it?" {
						t.Fatalf("prompt must identify the actual output: %q", options.Message)
					}
					return choice, nil
				})
				if calls != 1 || overwrite != (choice == "Yes") || (err == nil) != (choice == "Yes") {
					t.Fatalf("choice=%q: calls=%d, overwrite=%v, error=%v", choice, calls, overwrite, err)
				}
			}
			b, err := os.ReadFile(output)
			if err != nil || string(b) != "original" {
				t.Fatal("confirmation changed the existing file", err)
			}
		})
	}
}

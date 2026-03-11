package editor

import (
	"errors"
	"os"
	"os/exec"
)

func Open(path string) error {
	editor, err := resolveEditor()
	if err != nil {
		return err
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolveEditor() (string, error) {
	if editor := os.Getenv("GIT_EDITOR"); editor != "" {
		return editor, nil
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor, nil
	}
	if editor, err := exec.LookPath("micro"); err == nil {
		return editor, nil
	}
	if editor, err := exec.LookPath("vi"); err == nil {
		return editor, nil
	}
	return "", errors.New("no editor found")
}

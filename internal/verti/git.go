package verti

import (
	"errors"
	"os"
)

func ensureGitDir() error {
	info, err := os.Stat(".git")
	if err != nil || !info.IsDir() {
		return errors.New("missing .git")
	}
	return nil
}

package persistence

import "os"

func ensureBootstrapRoot(root string) error {
	if root == "" {
		return os.ErrInvalid
	}
	return os.MkdirAll(root, 0o700)
}

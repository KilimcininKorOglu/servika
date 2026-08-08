package apps

import (
	"errors"
	"path/filepath"
	"strings"
)

// refuseSymlinkEscape reports an error when target, or the nearest existing
// ancestor of it, resolves outside base.
//
// The walk upward is what makes this usable before the directory exists: an
// application can be registered against a path the tenant is about to create,
// but a symlink already planted on the way there still has to be caught.
func refuseSymlinkEscape(base, target string) error {
	check := target
	for check == base || strings.HasPrefix(check, base+"/") {
		real, err := filepath.EvalSymlinks(check)
		if err != nil {
			// This component does not exist yet; ask its parent.
			if check == base {
				return nil
			}
			check = filepath.Dir(check)
			continue
		}
		realBase, err := filepath.EvalSymlinks(base)
		if err != nil {
			realBase = base
		}
		if real != realBase && !strings.HasPrefix(real, realBase+"/") {
			return errors.New("application directory cannot leave the home directory through a symlink")
		}
		return nil
	}
	return errors.New("application directory cannot leave the home directory")
}

package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (r *Repository) InitSubmodule(ctx context.Context, expected SubmoduleInfo) error {
	return r.withRefEdit(ctx, func() error {
		if expected.Initialized || expected.IndexOID == "" {
			return fmt.Errorf("select an unregistered submodule with a staged gitlink")
		}
		// Hold the parent index stable while Git resolves a registered URL.
		lock := &checkoutIndexLock{release: func() {}}
		defer lock.close()
		if err := lock.open(ctx, r); err != nil {
			return err
		}
		if err := r.verifySubmodule(ctx, expected, false); err != nil {
			return err
		}
		return r.writeRefCommand(ctx, "--literal-pathspecs", "submodule", "init", "--", expected.Path)
	})
}

func (r *Repository) DeinitSubmodule(ctx context.Context, expected SubmoduleInfo) error {
	return r.withRefEdit(ctx, func() error {
		if !expected.Initialized {
			return fmt.Errorf("select a registered submodule")
		}
		common, err := os.OpenRoot(r.CommonDir)
		if err != nil {
			return err
		}
		defer common.Close()
		lock, err := lockGitStorageFile(common, "config")
		if err != nil {
			return err
		}
		defer lock.close()
		before, mode, err := readGitStorageFile(common, "config", remoteConfigLimit)
		if err != nil {
			return err
		}
		if err := r.verifySubmodule(ctx, expected, true); err != nil {
			return err
		}
		entries, err := r.readConfigValues(ctx, submoduleConfigPattern)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name, field, ok := submoduleConfigKey(entry.key)
			if ok && name == expected.Name && field == "url" && !r.isPrimaryConfig(entry.origin) {
				return fmt.Errorf("the submodule is registered outside the repository's config file")
			}
		}
		dir, err := os.MkdirTemp("", "gitwave-submodule-config-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		draft := filepath.Join(dir, "config")
		if err := os.WriteFile(draft, before, 0o600); err != nil {
			return err
		}
		// The old UI unregisters the configuration while retaining the checkout.
		if _, err := r.readLimited(ctx, "", 64<<10, "config", "--file", draft,
			"--remove-section", "submodule."+expected.Name); err != nil {
			return err
		}
		next, err := os.ReadFile(draft)
		if err != nil {
			return err
		}
		if err := lock.prepare(next, mode); err != nil {
			return err
		}
		if err := r.verifySubmodule(ctx, expected, true); err != nil {
			return err
		}
		current, currentMode, err := readGitStorageFile(common, "config", remoteConfigLimit)
		if err != nil || currentMode != mode || !bytes.Equal(current, before) {
			return fmt.Errorf("the Git config changed during submodule deactivation")
		}
		if err := r.verifyGitStorageLocks(ctx, common, lock); err != nil {
			return err
		}
		return lock.publish()
	})
}

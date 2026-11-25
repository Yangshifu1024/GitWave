package gitcli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
)

// ResolveCommit pins a revision supplied by the user to a full commit ID.
func (r *Repository) ResolveCommit(ctx context.Context, revision string) (string, error) {
	if revision == "" || len(revision) > 1024 || strings.ContainsRune(revision, 0) {
		return "", fmt.Errorf("invalid Git revision")
	}
	raw, err := r.readLimited(ctx, "", 128, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	oid := strings.TrimSpace(string(raw))
	if !validOID(oid) {
		return "", fmt.Errorf("Git did not resolve the revision to a commit")
	}
	return oid, nil
}

func (r *Repository) withRefEdit(ctx context.Context, edit func() error) error {
	unlock, err := lockIndexWriter(ctx, r.CommonDir)
	if err != nil {
		return err
	}
	defer unlock()
	fresh, err := OpenRoot(ctx, r.Path())
	if err != nil {
		return err
	}
	if !r.SameCheckout(fresh) {
		return fmt.Errorf("the checkout changed before editing its references")
	}
	return edit()
}

func (r *Repository) writeRefCommand(ctx context.Context, args ...string) error {
	hooks, err := os.MkdirTemp("", "gitwave-no-hooks-*")
	if err != nil {
		return err
	}
	defer os.Remove(hooks)
	_, err = r.readLimited(ctx, "", 64<<10, append([]string{"-c", "core.hooksPath=" + hooks}, args...)...)
	return err
}

// editDirectRef compares the object while Git holds the ref lock, then
// rejects a symbolic ref even if it currently resolves to the same object.
func (r *Repository) editDirectRef(ctx context.Context, ref, expected, next, reason string, verify func() error) error {
	hooks, err := os.MkdirTemp("", "gitwave-no-hooks-*")
	if err != nil {
		return err
	}
	defer os.Remove(hooks)
	statement := "option no-deref\x00delete " + ref + "\x00" + expected + "\x00"
	if next != "" {
		statement = "option no-deref\x00update " + ref + "\x00" + next + "\x00" + expected + "\x00"
	}
	args := []string{"-c", "core.hooksPath=" + hooks, "update-ref", "--stdin", "-z", "-m", reason}
	if ref == "refs/stash" && next != "" {
		args = append(args, "--create-reflog")
	}
	cmd := r.command(ctx, "", args...)
	_, _, err = runReferenceTransaction(cmd, statement, func() error {
		symbolic, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", ref)
		if err != nil {
			return err
		}
		if symbolic != "" {
			return fmt.Errorf("reference %q changed into a symbolic reference", ref)
		}
		if verify != nil {
			return verify()
		}
		return ctx.Err()
	})
	return err
}

func (r *Repository) checkedRefName(ctx context.Context, namespace, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsRune(name, 0) ||
		(namespace == "heads" && name == "HEAD") {
		return "", fmt.Errorf("invalid Git reference name: %q", name)
	}
	ref := "refs/" + namespace + "/" + name
	if _, err := r.readLimited(ctx, "", 4096, "check-ref-format", ref); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("invalid Git reference name: %q", name)
	}
	return ref, nil
}

func findRef(refs References, name string) Reference {
	index, found := slices.BinarySearchFunc(refs.Refs, name, func(ref Reference, target string) int {
		return strings.Compare(ref.Name, target)
	})
	if !found {
		return Reference{}
	}
	return refs.Refs[index]
}

func checkRefTip(refs References, name, want string) error {
	ref := findRef(refs, name)
	if ref.OID != want || ref.Symbolic != "" {
		return fmt.Errorf("reference %q moved or changed since it was shown", name)
	}
	return nil
}

func (r *Repository) requireCommit(ctx context.Context, oid string) error {
	if !validOID(oid) {
		return fmt.Errorf("select a valid commit")
	}
	tip, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", oid+"^{commit}")
	if err != nil {
		return err
	}
	if tip != oid {
		return fmt.Errorf("commit %s is no longer available", oid)
	}
	return nil
}

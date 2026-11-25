package gitcli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CreateTag uses an empty expected object for creation and the explicitly
// confirmed tag object when replacing a tag.
func (r *Repository) CreateTag(ctx context.Context, name, base, annotation, expected string) error {
	annotation = strings.TrimSpace(annotation)
	if strings.ContainsRune(annotation, 0) || (expected != "" && !validOID(expected)) {
		return fmt.Errorf("invalid tag contents")
	}
	return r.withRefEdit(ctx, func() error {
		full, err := r.checkedRefName(ctx, "tags", name)
		if err != nil {
			return err
		}
		if err := r.requireCommit(ctx, base); err != nil {
			return err
		}
		refs, err := r.References(ctx)
		if err != nil {
			return err
		}
		if err := checkRefTip(refs, full, expected); err != nil {
			return err
		}
		oid := base
		if annotation != "" {
			oid, err = r.annotatedTag(ctx, name, base, annotation)
			if err != nil {
				return err
			}
		}
		if expected == "" {
			expected = strings.Repeat("0", len(oid))
		}
		return r.editDirectRef(ctx, full, expected, oid, "tag: "+name, nil)
	})
}

func (r *Repository) DeleteTag(ctx context.Context, name, expected string) error {
	if !validOID(expected) {
		return fmt.Errorf("select an existing tag")
	}
	return r.withRefEdit(ctx, func() error {
		full, err := r.checkedRefName(ctx, "tags", name)
		if err != nil {
			return err
		}
		refs, err := r.References(ctx)
		if err != nil {
			return err
		}
		if err := checkRefTip(refs, full, expected); err != nil {
			return err
		}
		return r.editDirectRef(ctx, full, expected, "", "tag: delete "+name, nil)
	})
}

func (r *Repository) annotatedTag(ctx context.Context, name, base, message string) (string, error) {
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return "", err
	}
	cmd := r.command(ctx, "", "var", "GIT_COMMITTER_IDENT")
	withRefIdentity(cmd, identity)
	output, err := readGitOutput(ctx, cmd, 16<<10, "var GIT_COMMITTER_IDENT")
	if err != nil {
		return "", err
	}
	tagger := strings.TrimRight(string(output), "\r\n")
	if tagger == "" || strings.ContainsAny(tagger, "\r\n") {
		return "", fmt.Errorf("invalid tagger signature")
	}
	input := fmt.Sprintf("object %s\ntype commit\ntag %s\ntagger %s\n\n%s\n", base, name, tagger, message)
	output, err = r.readLimited(ctx, input, 4096, "mktag")
	if err != nil {
		return "", err
	}
	oid := strings.TrimSpace(string(output))
	if !validOID(oid) {
		return "", fmt.Errorf("Git returned an invalid tag object ID")
	}
	return oid, nil
}

func withRefIdentity(cmd *exec.Cmd, identity commitIdentity) {
	cmd.Env = append(cmd.Env, "GIT_COMMITTER_NAME="+identity.name, "GIT_COMMITTER_EMAIL="+identity.email)
}

func (r *Repository) TagAnnotation(ctx context.Context, oid string) (string, error) {
	if !validOID(oid) {
		return "", fmt.Errorf("invalid Git tag object")
	}
	out, err := r.readLimited(ctx, "", 1<<20, "cat-file", "tag", oid)
	if err != nil {
		return "", err
	}
	_, message, found := strings.Cut(string(out), "\n\n")
	if !found {
		return "", fmt.Errorf("invalid Git tag message")
	}
	return strings.TrimRight(message, "\r\n"), nil
}

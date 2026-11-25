package gitcli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

func (r *Repository) publishCommit(ctx context.Context, lock *checkoutIndexLock, gitDir string, shown Status,
	head, oid, message string, committer commitIdentity, interactive *interactiveAdvance) error {
	target := shown.HeadRef
	if target == "" {
		target = "HEAD"
	}
	old := head
	if old == "" {
		old = strings.Repeat("0", len(oid))
	}
	cmd := r.command(ctx, "", "-c", "core.hooksPath="+gitDir, "update-ref", "--stdin", "-z", "--create-reflog", "-m", message)
	withRefIdentity(cmd, committer)
	verify := func() error {
		if !lock.ownsLock() {
			return fmt.Errorf("the Git index lock was replaced before committing")
		}
		if err := r.checkIndexCheckout(ctx); err != nil {
			return err
		}
		ref, err := r.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
		if err != nil {
			return err
		}
		tip, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil {
			return err
		}
		operation, err := r.readCommitState(ctx, lock.root)
		if err != nil {
			return err
		}
		if tip != head || ref != shown.HeadRef || operation != shown.Commit {
			return fmt.Errorf("HEAD or the repository operation changed before publishing the commit")
		}
		return ctx.Err()
	}

	// Dereferencing HEAD locks its attachment and the branch it names.
	statement := "update HEAD\x00" + oid + "\x00" + old + "\x00"
	if interactive != nil {
		statement += interactive.statement(len(old))
	}
	if shown.Commit.MergeHeads != "" {
		autoMerge, err := r.exactRootRef(ctx, "AUTO_MERGE")
		if err != nil {
			return err
		}
		if autoMerge != "" {
			statement += deleteMergeTree(autoMerge)
		}
	}
	published, sentCommit, txErr := runReferenceTransaction(cmd, statement, verify)
	if published {
		return nil
	}
	if sentCommit {
		// A cancelled child may have finished the atomic update before its
		// acknowledgement reached Go. Recheck the captured target only.
		check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if actual, err := r.optionalRef(check, "rev-parse", "--verify", "--quiet", target); err == nil && actual == oid {
			if interactive == nil {
				return nil
			}
			if pause, err := r.interactiveRebaseRef(check); err == nil && pause == interactive.after {
				return nil
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return txErr
}

func runReferenceTransaction(cmd *exec.Cmd, statement string, verify func() error) (bool, bool, error) {
	cmd.Stdin = nil
	input, err := cmd.StdinPipe()
	if err != nil {
		return false, false, err
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return false, false, err
	}
	defer output.Close()
	var stderr gitDiagnostic
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return false, false, err
	}
	ack := bufio.NewScanner(output)
	ack.Buffer(make([]byte, 256), 4096)
	published, sent, txErr := finishReferenceTransaction(input, ack, statement, verify)
	_ = input.Close()
	waitErr := cmd.Wait()
	if published {
		return true, true, nil
	}
	if diagnostic := strings.TrimSpace(stderr.buffer.String()); diagnostic != "" {
		return false, sent, fmt.Errorf("update Git reference: %s", diagnostic)
	}
	if txErr != nil {
		return false, sent, txErr
	}
	return false, sent, waitErr
}

func finishReferenceTransaction(input io.Writer, ack *bufio.Scanner, statement string, verify func() error) (published, sent bool, err error) {
	if _, err := io.WriteString(input, "start\x00"+statement+"prepare\x00"); err != nil {
		return false, false, err
	}
	for _, expected := range []string{"start: ok", "prepare: ok"} {
		if !ack.Scan() || ack.Text() != expected {
			return false, false, fmt.Errorf("Git could not prepare the reference transaction")
		}
	}
	if err := verify(); err != nil {
		return false, false, err
	}
	if _, err := io.WriteString(input, "commit\x00"); err != nil {
		return false, false, err
	}
	if !ack.Scan() || ack.Text() != "commit: ok" {
		return false, true, fmt.Errorf("Git did not confirm the reference update")
	}
	return true, true, nil
}

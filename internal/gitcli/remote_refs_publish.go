package gitcli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type remoteRefMove struct {
	from remoteTrackingRef
	to   remoteTrackingRef
}

func (s remoteConfigEdit) trackingRefTransaction() (string, map[string]bool) {
	var statement strings.Builder
	affected := make(map[string]bool, len(s.deletes)+len(s.moves))
	deleted := make(map[string]bool, len(s.deletes))
	for _, ref := range s.deletes {
		affected[ref.name], deleted[ref.name] = true, true
		statement.WriteString("option no-deref\x00")
		if ref.symbolic != "" {
			fmt.Fprintf(&statement, "symref-delete %s\x00%s\x00", ref.name, ref.symbolic)
		} else {
			fmt.Fprintf(&statement, "delete %s\x00%s\x00", ref.name, ref.oid)
		}
	}
	for _, pair := range s.moves {
		affected[pair.to.name] = true
		statement.WriteString("option no-deref\x00")
		if pair.to.symbolic != "" {
			fmt.Fprintf(&statement, "symref-create %s\x00%s\x00", pair.to.name, pair.to.symbolic)
		} else {
			fmt.Fprintf(&statement, "create %s\x00%s\x00", pair.to.name, pair.to.oid)
		}
		if !deleted[pair.from.name] {
			if pair.from.symbolic != "" {
				fmt.Fprintf(&statement, "option no-deref\x00symref-verify %s\x00%s\x00", pair.from.name, pair.from.symbolic)
			} else {
				statement.WriteString(verifyCheckoutRef(pair.from.name, pair.from.oid))
			}
		}
	}
	return statement.String(), affected
}

func (s remoteConfigEdit) publishTrackingChanges(configBefore, configAfter []byte, publishConfig func() error) error {
	statement, affected := s.trackingRefTransaction()
	r := s.repo
	if err := r.protectFetchHeads(s.ctx, affected); err != nil {
		return err
	}
	if err := r.checkTrackingRefPaths(s.ctx, affected); err != nil {
		return err
	}
	finalize, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 30*time.Second)
	defer cancel()
	cmd := r.command(finalize, "", "-c", "core.hooksPath="+filepath.Dir(s.path), "update-ref", "--stdin", "-z", "-m", "remote: update tracking references")
	configPublished := false
	logs := remoteReflogCopies{}
	defer logs.close()
	committed, sent, err := runReferenceTransaction(cmd, statement, func() error {
		if err := r.protectFetchHeads(s.ctx, affected); err != nil {
			return err
		}
		if err := r.checkTrackingRefPaths(s.ctx, affected); err != nil {
			return err
		}
		for _, ref := range s.deletes {
			if !r.remoteRefMatches(s.ctx, ref) {
				return fmt.Errorf("the remote-tracking reference %q changed", ref.name)
			}
		}
		for _, move := range s.moves {
			if !r.remoteRefMatches(s.ctx, move.from) || !r.remoteRefMatches(s.ctx, remoteTrackingRef{name: move.to.name}) {
				return fmt.Errorf("the remote-tracking references changed before moving %q", move.from.name)
			}
		}
		if err := logs.install(s.ctx, r, s.moves); err != nil {
			return err
		}
		if err := publishConfig(); err != nil {
			return err
		}
		configPublished = true
		return nil
	})
	if committed {
		logs.published = true
		return nil
	}
	if !configPublished {
		return err
	}
	if sent {
		before, after := true, true
		for _, ref := range s.deletes {
			before = before && r.remoteRefMatches(finalize, ref)
			after = after && r.remoteRefMatches(finalize, remoteTrackingRef{name: ref.name})
		}
		for _, move := range s.moves {
			before = before && r.remoteRefMatches(finalize, remoteTrackingRef{name: move.to.name})
			after = after && r.remoteRefMatches(finalize, move.to)
		}
		if after {
			logs.published = true
			return nil
		}
		if !before {
			logs.published = true
			return errors.Join(err, fmt.Errorf("some tracking references changed while publishing; reload the remote list"))
		}
	}
	return errors.Join(err, r.restoreRemoteConfig(configBefore, configAfter))
}

func (r *Repository) remoteRefMatches(ctx context.Context, want remoteTrackingRef) bool {
	symbolic, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", want.name)
	if err != nil || symbolic != want.symbolic {
		return false
	}
	if want.symbolic != "" {
		return true
	}
	oid, err := r.exactRootRef(ctx, want.name)
	return err == nil && oid == want.oid
}

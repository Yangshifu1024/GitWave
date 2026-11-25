package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"gitwave.work/internal/proxy"
)

type pushDestination struct {
	stage               *transportRepo
	transport           *transportAttempt
	media               *transportAttempt
	name                string
	scratch             string
	lfsURL, lfsProtocol string
	lfsConfigError      string
	local               localLFSTransport
}

func (d pushDestination) readTips(ctx context.Context, refs []CheckoutTarget) (map[string]string, error) {
	found := make(map[string]string, len(refs))
	for batch := range pushRefBatches(refs) {
		if err := d.readTipBatch(ctx, batch, found); err != nil {
			return nil, err
		}
	}
	return found, nil
}

func (d pushDestination) readTipBatch(ctx context.Context, refs []CheckoutTarget, found map[string]string) error {
	args := append([]string{}, d.transport.options...)
	args = append(args, "ls-remote", "--refs", "--", d.name)
	wanted := make(map[string]bool, len(refs))
	for _, ref := range refs {
		args = append(args, ref.Ref)
		wanted[ref.Ref] = true
	}
	cmd := d.stage.command(ctx, "", args...)
	output := pushLines{parse: func(line string) error {
		oid, name, ok := strings.Cut(line, "\t")
		if !ok || !validOID(oid) {
			return fmt.Errorf("invalid remote reference in force push review")
		}
		// ls-remote can also match a suffix of a longer reference name.
		if wanted[name] {
			if found[name] != "" {
				return fmt.Errorf("duplicate remote reference in force push review")
			}
			found[name] = oid
		}
		return nil
	}}
	cmd.Stdout = &output
	if err := d.transport.run(ctx, cmd, "ls-remote", d.stage.proxyURLs...); err != nil {
		return err
	}
	return output.result()
}

type pushReport struct {
	accepted []string
	skipped  []string
}

func (d pushDestination) push(ctx context.Context, refs []CheckoutTarget, leases map[string]string) (pushReport, error) {
	var report pushReport
	for batch := range pushRefBatches(refs) {
		result, err := d.pushBatch(ctx, batch, leases)
		report.accepted = append(report.accepted, result.accepted...)
		report.skipped = append(report.skipped, result.skipped...)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func (d pushDestination) pushBatch(ctx context.Context, refs []CheckoutTarget, leases map[string]string) (pushReport, error) {
	args := append([]string{}, d.transport.options...)
	args = append(args, "-c", "push.pushOption=", "push", "--porcelain", "--progress", "--no-follow-tags",
		"--no-mirror", "--signed=no", "--recurse-submodules=no", "--receive-pack=git-receive-pack")
	var hookEnv []string
	if slices.ContainsFunc(refs, func(ref CheckoutTarget) bool { return ref.OID != "" }) {
		var err error
		hookEnv, err = d.lfsHookEnvironment(ctx)
		if err != nil {
			return pushReport{}, err
		}
	} else {
		args = append(args, "--no-verify")
	}
	if leases != nil {
		for _, target := range refs {
			args = append(args, "--force-with-lease="+target.Ref+":"+leases[target.Ref])
		}
	}
	args = append(args, "--", d.name)
	for _, target := range refs {
		if target.OID == "" && (!strings.HasPrefix(target.Ref, "refs/heads/") || !validOID(leases[target.Ref])) {
			return pushReport{}, fmt.Errorf("deleting a remote branch requires an explicit lease")
		}
		// The object was confirmed by the user. A concurrent local ref
		// move cannot cause Git to send a different commit or tag.
		args = append(args, target.OID+":"+target.Ref)
	}
	var report pushReport
	var failures []string
	wanted := make(map[string]bool, len(refs))
	for _, ref := range refs {
		wanted[ref.Ref] = ref.OID == ""
	}
	output := pushLines{parse: func(line string) error {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || len(fields[0]) != 1 {
			return nil
		}
		_, name, ok := strings.Cut(fields[1], ":")
		deleting, known := wanted[name]
		if !ok || !known {
			return fmt.Errorf("Git reported an unexpected push destination")
		}
		delete(wanted, name)
		switch fields[0] {
		case " ", "*", "+", "=":
			if deleting {
				return fmt.Errorf("Git did not report a deletion for %q", name)
			}
			report.accepted = append(report.accepted, name)
		case "-":
			if !deleting {
				return fmt.Errorf("Git reported an unexpected deletion for %q", name)
			}
			report.accepted = append(report.accepted, name)
		case "!":
			reason := fields[2]
			if leases == nil && strings.HasPrefix(name, "refs/tags/") && strings.HasPrefix(reason, "[rejected]") &&
				strings.Contains(reason, "(already exists)") {
				report.skipped = append(report.skipped, name)
			} else {
				failures = append(failures, name+" "+reason)
			}
		default:
			return fmt.Errorf("Git reported an unexpected push status for %q", name)
		}
		return nil
	}}
	cmd := d.stage.command(ctx, "", args...)
	cmd.Env = append(cmd.Env, hookEnv...)
	if len(hookEnv) != 0 && d.transport.protocol == "file" && (d.lfsProtocol == "http" || d.lfsProtocol == "https") {
		// A local Git destination may still have an independent network media store.
		cmd.Env = proxy.CommandEnvironment(ctx, cmd.Env)
	}
	cmd.Stdout = &output
	err := d.transport.run(ctx, cmd, "push", d.stage.proxyURLs...)
	if errors.Is(err, ErrHTTPSAuthentication) || errors.Is(err, errLFSPushAuthentication) || errors.Is(err, context.Canceled) {
		return report, err
	}
	statusErr := output.result()
	if statusErr == nil && len(failures) != 0 {
		statusErr = fmt.Errorf("push rejected: %s", strings.Join(failures, "; "))
	}
	if statusErr != nil {
		if d.transport.auth != nil {
			statusErr = errors.New(strings.ReplaceAll(statusErr.Error(), d.transport.auth.Password, "[redacted]"))
		}
		return report, errors.New(redactProxyDiagnostic(statusErr.Error(), cmd.Env, d.stage.proxyURLs))
	}
	// Git exits nonzero for individual tag collisions even when all
	// other refs were accepted. The porcelain report names those tags.
	if len(report.accepted)+len(report.skipped) == len(refs) && (err == nil || len(report.skipped) != 0) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	return report, fmt.Errorf("Git did not confirm all requested push updates")
}

// A forced update uses two ref names and up to two SHA-256 OIDs per ref.
// Leave room for fixed arguments within Windows' command-line limit.
func pushRefBatches(refs []CheckoutTarget) iter.Seq[[]CheckoutTarget] {
	return func(yield func([]CheckoutTarget) bool) {
		start, size := 0, 0
		for i, ref := range refs {
			cost := 2*len(ref.Ref) + 160
			if i > start && size+cost > 20<<10 {
				if !yield(refs[start:i]) {
					return
				}
				start, size = i, 0
			}
			size += cost
		}
		if start < len(refs) {
			yield(refs[start:])
		}
	}
}

// Bound individual protocol records independently of the number of refs.
type pushLines struct {
	line  bytes.Buffer
	parse func(string) error
	err   error
}

func (o *pushLines) Write(data []byte) (int, error) {
	size := len(data)
	for len(data) > 0 && o.err == nil {
		line, rest, complete := bytes.Cut(data, []byte{'\n'})
		if o.line.Len()+len(line) > 128<<10 {
			o.err = errGitOutputLimit
			break
		}
		o.line.Write(line)
		if complete {
			o.err = o.parse(strings.TrimSuffix(o.line.String(), "\r"))
			o.line.Reset()
		}
		data = rest
	}
	return size, nil
}

func (o *pushLines) result() error {
	if o.err == nil && o.line.Len() != 0 {
		return fmt.Errorf("incomplete Git reference status")
	}
	return o.err
}

package gitcli

import (
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSSHLoopbackCloneFetchAndPush(t *testing.T) {
	source := os.Getenv("GITWAVE_TEST_SSH_GIT_URL")
	if source == "" {
		t.Skip("enable an isolated OpenSSH Git loopback fixture")
	}
	address, err := url.Parse(source)
	if err != nil || address.Scheme != "ssh" || address.Port() == "" {
		t.Fatal("the Git fixture must use SSH on an explicit loopback port")
	}
	ip, err := netip.ParseAddr(address.Hostname())
	if err != nil || !ip.IsLoopback() {
		t.Fatal("the Git fixture must use a loopback address")
	}
	ctx := t.Context()
	target, err := PrepareClone(ctx, filepath.Join(t.TempDir(), "clone"))
	if err != nil {
		t.Fatal(err)
	}
	clone, err := target.Clone(ctx, source, false, CloneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dir := clone.Repo.Path()
	fixtureGit(t, dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
		"commit", "--allow-empty", "-m", "Pushed through the loaded agent key")
	options := pushOptions(t, dir, clone.Repo)
	plan, err := clone.Repo.PreparePush(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if pushed, err := plan.Apply(ctx, FetchOptions{}, nil); err != nil || pushed.Branches != 1 {
		t.Fatalf("SSH push = %+v, %v", pushed, err)
	}
	if fetched, err := clone.Repo.Fetch(ctx, FetchOptions{}); err != nil || len(fetched.Completed) != 1 {
		t.Fatalf("SSH fetch = %+v, %v", fetched, err)
	}
	if got := gitValue(t, dir, "rev-parse", "refs/remotes/origin/main"); got != options.Branch.OID {
		t.Fatalf("remote branch = %s, want %s", got, options.Branch.OID)
	}
}

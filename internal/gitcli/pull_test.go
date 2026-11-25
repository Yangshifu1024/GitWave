package gitcli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func pullFixture(t *testing.T, initOptions ...string) (string, string, *Repository, PullOptions) {
	t.Helper()
	source := newFixture(t, initOptions...)
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", source, ".")
	fixtureGit(t, client, "config", "user.name", "Pull Fixture")
	fixtureGit(t, client, "config", "user.email", "pull@example.invalid")
	repo := openFixture(t, client)
	options := PullOptions{Remote: "origin", Branch: "main",
		Local: CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, client, "rev-parse", "HEAD")}}
	return source, client, repo, options
}

func TestPullFastForwardsAttachedBranchToSelectedRemoteBranch(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	fixtureGit(t, source, "switch", "-c", "topic/nested")
	writeFixture(t, source, "remote.txt", "selected remote contents\n")
	fixtureGit(t, source, "add", "remote.txt")
	fixtureGit(t, source, "commit", "-m", "Advance chosen branch")
	target := gitValue(t, source, "rev-parse", "HEAD")
	options.Branch, options.Rebase = "topic/nested", true
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || !result.Fetched || result.Rebased || result.UpToDate || result.OID != target || result.Warning != "" {
		t.Fatalf("fast-forward did not land the fetched commit: %+v, %v", result, err)
	}
	if status := readFixtureStatus(t, repo); status.HeadRef != "refs/heads/main" || status.HeadOID != target ||
		status.Upstream != "origin/main" || len(status.Changes) != 0 {
		t.Fatalf("pull changed the attachment or upstream: %+v", status)
	}
	if contents, err := os.ReadFile(filepath.Join(client, "remote.txt")); err != nil || string(contents) != "selected remote contents\n" {
		t.Fatalf("fast-forward did not materialize remote data: %q, %v", contents, err)
	}
}

func TestPullChoicesUseTheSourceNameForARenamedTrackingBranch(t *testing.T) {
	source, client, repo, _ := pullFixture(t)
	fixtureGit(t, source, "branch", "release")
	fixtureGit(t, client, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/release:refs/remotes/origin/mirror")
	fixtureGit(t, client, "config", "branch.main.remote", "origin")
	fixtureGit(t, client, "config", "branch.main.merge", "refs/heads/release")
	if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: "origin"}); err != nil {
		t.Fatal(err)
	}
	choices, err := repo.PullChoices(t.Context(), "main")
	if err != nil || choices.UpstreamRemote != "origin" || choices.UpstreamBranch != "release" ||
		len(choices.Remotes) != 1 || !slices.Equal(choices.Remotes[0].Branches, []string{"release"}) {
		t.Fatalf("pull offered a local alias instead of its remote source: %+v, %v", choices, err)
	}
}

func TestPullRefusesAnotherSourceMappedToTheSelectedTrackingRef(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	fixtureGit(t, source, "switch", "-c", "other/main")
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "From the unselected branch")
	fixtureGit(t, client, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/group/*:refs/remotes/origin/*")
	fixtureGit(t, client, "config", "--add", "remote.origin.fetch", "+refs/heads/other/*:refs/remotes/origin/*")
	options.Branch = "group/main"
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || result.Fetched {
		t.Fatalf("Pull used another source to stand in for the missing branch: %+v, %v", result, err)
	}
	if head := gitValue(t, client, "rev-parse", "HEAD"); head != options.Local.OID {
		t.Fatalf("Pull used the unselected source: %s", head)
	}
}

func TestPullCanPopulateAnUnbornBranch(t *testing.T) {
	source := newFixture(t)
	client := t.TempDir()
	fixtureGit(t, client, "init", "--quiet", "-b", "main")
	fixtureGit(t, client, "remote", "add", "origin", source)
	repo := openFixture(t, client)
	options := PullOptions{Remote: "origin", Branch: "main", Local: CheckoutTarget{Ref: "refs/heads/main"}}
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || result.OID != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("pull did not populate the unborn branch: %+v, %v", result, err)
	}
	if status := readFixtureStatus(t, repo); status.HeadRef != "refs/heads/main" || status.HeadOID != result.OID ||
		len(status.Changes) != 0 {
		t.Fatalf("pull left an incomplete unborn checkout: %+v", status)
	}
}

func TestPullKeepsCurrentBranchOnDivergenceAndRebasesWhenRequested(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	writeFixture(t, client, "local.txt", "unique local contents\n")
	fixtureGit(t, client, "add", "local.txt")
	fixtureGit(t, client, "commit", "-m", "Keep local contribution")
	local := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "branch", "preserve-local")
	writeFixture(t, source, "remote.txt", "unique remote contents\n")
	fixtureGit(t, source, "add", "remote.txt")
	fixtureGit(t, source, "commit", "-m", "Remote contribution")
	target := gitValue(t, source, "rev-parse", "HEAD")
	options.Local.OID = local
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || !result.Fetched ||
		!strings.Contains(err.Error(), "require a merge") {
		t.Fatalf("divergence was not refused: %+v, %v", result, err)
	}
	if got := gitValue(t, client, "rev-parse", "HEAD"); got != local {
		t.Fatalf("plain pull moved the local branch: %s", got)
	}
	options.Rebase = true
	plan, err = repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || !result.Rebased || result.OID == local || result.OID == target {
		t.Fatalf("rebase did not replay a local commit: %+v, %v", result, err)
	}
	if got := gitValue(t, client, "rev-parse", "HEAD^"); got != target {
		t.Fatalf("rebased parent = %s, want %s", got, target)
	}
	if got := gitValue(t, client, "rev-parse", "preserve-local"); got != local {
		t.Fatalf("private rebase rewrote a different local branch: %s", got)
	}
	for file, want := range map[string]string{"local.txt": "unique local contents\n", "remote.txt": "unique remote contents\n"} {
		if contents, err := os.ReadFile(filepath.Join(client, file)); err != nil || string(contents) != want {
			t.Fatalf("rebased file %s = %q, %v", file, contents, err)
		}
	}
	if got := gitValue(t, client, "show", "-s", "--format=%an <%ae> %s", "HEAD"); got != "Pull Fixture <pull@example.invalid> Keep local contribution" {
		t.Fatalf("rebase changed original author or subject: %q", got)
	}
}

func TestPullRebaseConflictsStayInPrivateCheckout(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	for path, contents := range map[string]string{client: "local version\n", source: "remote version\n"} {
		writeFixture(t, path, "conflict.txt", contents)
		fixtureGit(t, path, "add", "conflict.txt")
		fixtureGit(t, path, "commit", "-m", "Modify the same file")
	}
	index := filepath.Join(repo.GitDir, "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	options.Local.OID = gitValue(t, client, "rev-parse", "HEAD")
	options.Rebase = true
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	var conflict *PullRebaseConflict
	if !errors.As(err, &conflict) || len(conflict.Paths) != 1 || conflict.Paths[0] != "conflict.txt" || !result.Fetched {
		t.Fatalf("expected the isolated rebase conflict: %+v, %v", result, err)
	}
	if got := gitValue(t, client, "rev-parse", "HEAD"); got != options.Local.OID {
		t.Fatalf("private conflict moved the live branch to %s", got)
	}
	if after, err := os.ReadFile(index); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("private conflict changed the live index: %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(client, "conflict.txt")); err != nil || string(contents) != "local version\n" {
		t.Fatalf("private rebase changed the live file: %q, %v", contents, err)
	}
	if state, err := repo.commitState(t.Context()); err != nil || state.Continuation != "" {
		t.Fatalf("private conflict left a live continuation: %+v, %v", state, err)
	}
}

func TestPullStashReplaysStagedUnstagedAndUntrackedChanges(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Remote advance")
	writeFixture(t, client, "mixed.txt", "staged work\n")
	fixtureGit(t, client, "add", "mixed.txt")
	writeFixture(t, client, "mixed.txt", "later edit\n")
	writeFixture(t, client, "untracked.txt", "local file\n")
	options.Stash = true
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || result.Warning != "" || result.OID != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("pull with stash failed: %+v, %v", result, err)
	}
	if staged := gitValue(t, client, "show", ":mixed.txt"); staged != "staged work" {
		t.Fatalf("pull restored a different index: %q", staged)
	}
	for file, want := range map[string]string{"mixed.txt": "later edit\n", "untracked.txt": "local file\n"} {
		if contents, err := os.ReadFile(filepath.Join(client, file)); err != nil || string(contents) != want {
			t.Fatalf("replayed file %s = %q, %v", file, contents, err)
		}
	}
	if list := gitValue(t, client, "stash", "list"); list != "" {
		t.Fatalf("successful pull left an extra stash: %q", list)
	}
}

func TestPullStashRefusesChangesMadeAfterConfirmation(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Remote advance")
	writeFixture(t, client, "mixed.txt", "confirmed change\n")
	options.Stash = true
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, client, "mixed.txt", "edit after confirmation\n")
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("the pull used a stale confirmation: %v", err)
	}
	if got := gitValue(t, client, "rev-parse", "HEAD"); got != options.Local.OID {
		t.Fatalf("stale pull moved the HEAD: %s", got)
	}
	if contents, err := os.ReadFile(filepath.Join(client, "mixed.txt")); err != nil || string(contents) != "edit after confirmation\n" {
		t.Fatalf("the new edit was changed: %q, %v", contents, err)
	}
}

func TestPullAuthRetryDoesNotAuthorizeChangesMadeAtThePrompt(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Authenticated update")
	fixtureGit(t, source, "update-server-info")
	const username, token = "fixture-user", "fixture-token"
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(filepath.Join(source, ".git"))))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != username || password != token {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave test\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, source, server)
	fixtureGit(t, client, "remote", "set-url", "origin", server.URL+"/project.git")
	writeFixture(t, client, "mixed.txt", "confirmed edit\n")
	options.Stash = true
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	var challenge *RemoteAuthError
	if _, err := plan.Apply(t.Context(), FetchOptions{}); !errors.As(err, &challenge) {
		t.Fatalf("pull did not request the HTTPS credentials: %v", err)
	}
	writeFixture(t, client, "mixed.txt", "edit while the credential prompt was open\n")
	result, err := plan.Apply(t.Context(), FetchOptions{Challenge: challenge,
		Credentials: &HTTPSCredential{Username: username, Password: token}})
	if err == nil || !result.Fetched || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("authentication retry accepted a stale stash confirmation: %+v, %v", result, err)
	}
	if got := gitValue(t, client, "rev-parse", "HEAD"); got != options.Local.OID {
		t.Fatalf("stale Pull authentication retry moved HEAD to %s", got)
	}
	if contents, err := os.ReadFile(filepath.Join(client, "mixed.txt")); err != nil || string(contents) != "edit while the credential prompt was open\n" {
		t.Fatalf("authentication retry touched the latest edit: %q, %v", contents, err)
	}
}

func TestPullStashConflictKeepsTheRecoveryEntry(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	writeFixture(t, source, "conflict.txt", "new upstream file contents\n")
	fixtureGit(t, source, "add", "conflict.txt")
	fixtureGit(t, source, "commit", "-m", "Overlap the local working change")
	writeFixture(t, client, "conflict.txt", "uncommitted local contents\n")
	options.Stash = true
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || !strings.Contains(result.Warning, "stash list") ||
		result.OID != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("pull did not report the recovered stash: %+v, %v", result, err)
	}
	if got := gitValue(t, client, "show", "stash@{0}:conflict.txt"); got != "uncommitted local contents" {
		t.Fatalf("stash lost the working contents: %q", got)
	}
	if stages := gitValue(t, client, "ls-files", "--unmerged", "--", "conflict.txt"); stages == "" {
		t.Fatal("stash conflict did not produce conflict stages")
	}
}

func TestPullRefusesAnUnmergedIndexBeforeFetching(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	blob := gitValue(t, client, "rev-parse", "HEAD:conflict.txt")
	fixtureGit(t, client, "update-index", "--force-remove", "conflict.txt")
	cmd := fixtureCommand(t, client, "update-index", "--index-info")
	cmd.Stdin = strings.NewReader("100644 " + blob + " 1\tconflict.txt\n100644 " + blob + " 2\tconflict.txt\n")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("could not create an unmerged entry: %s %v", output, err)
	}
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Should not be fetched")
	if _, err := repo.PreparePull(t.Context(), options); err == nil || !strings.Contains(err.Error(), "index conflicts") {
		t.Fatalf("unmerged pull was not refused: %v", err)
	}
	if got := gitValue(t, client, "rev-parse", "origin/main"); got != options.Local.OID {
		t.Fatalf("refused pull fetched to %s", got)
	}
}

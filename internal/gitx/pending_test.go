package gitx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/t1mdotcom/lazytuck/internal/testutil"
)

func TestPendingDiffMatchesAddAllWithoutTouchingIndex(t *testing.T) {
	repo, _ := setup(t)
	writeFile(t, filepath.Join(repo, "Configs", "zsh", ".zshrc"), "v1\nexport TOKEN=abc\n")
	writeFile(t, filepath.Join(repo, "Configs", "npm", ".npmrc"), "_password=secret\n")
	testutil.Git(t, repo, "add", "Configs/zsh/.zshrc") // a partially staged index must survive
	indexBefore := testutil.Git(t, repo, "diff", "--cached", "--name-only")
	statusBefore := testutil.Git(t, repo, "status", "--porcelain")

	d, err := PendingDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"+export TOKEN=abc", "+_password=secret", "+++ b/Configs/npm/.npmrc"} {
		if !strings.Contains(d, want) {
			t.Errorf("pending diff lacks %q:\n%s", want, d)
		}
	}
	if got := testutil.Git(t, repo, "diff", "--cached", "--name-only"); got != indexBefore {
		t.Errorf("real index changed: %q → %q", indexBefore, got)
	}
	if got := testutil.Git(t, repo, "status", "--porcelain"); got != statusBefore {
		t.Errorf("status changed: %q → %q", statusBefore, got)
	}
}

func TestOutgoingDiffSeesSecretsRemovedLater(t *testing.T) {
	repo, _ := setup(t)
	writeFile(t, filepath.Join(repo, "Configs", "zsh", ".zshrc"), "v1\nexport TOKEN=abc\n")
	testutil.CommitAll(t, repo, "add token")
	writeFile(t, filepath.Join(repo, "Configs", "zsh", ".zshrc"), "v1\n")
	testutil.CommitAll(t, repo, "remove token")

	d, err := OutgoingDiff(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "+export TOKEN=abc") {
		t.Errorf("outgoing history should still contain the added secret:\n%s", d)
	}
}

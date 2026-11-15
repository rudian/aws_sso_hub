package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
)

// FU-04b: in a blob:none clone, rename detection needs a blob that is not on disk. The reader may not fetch it,
// so the first status fails; the retry runs without rename detection and marks the result.
func TestStatusInPartialCloneRetriesWithoutRenames(t *testing.T) {
	sb := gitxtest.New(t)
	ctx := context.Background()
	work := sb.InitRepo(t, "pc-work", nil)
	var body strings.Builder
	for i := 0; i < 40; i++ {
		body.WriteString("line of text number " + strings.Repeat("x", i) + "\n")
	}
	sb.CommitFile(t, work, "old.txt", body.String())
	bare := filepath.Join(sb.Root, "pc.git")
	plainGit(t, sb, sb.Root, "clone", "-q", "--bare", work, bare)
	plainGit(t, sb, bare, "config", "uploadpack.allowFilter", "true")
	plainGit(t, sb, bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	repo := filepath.Join(sb.Root, "pc")
	plainGit(t, sb, sb.Root, "-c", "protocol.file.allow=always", "clone", "-q", "--no-checkout", "--filter=blob:none", "file://"+bare, repo)
	plainGit(t, sb, repo, "read-tree", "HEAD")
	// Stage "old.txt -> new.txt" with similar (not identical) content: telling it from a delete plus an add
	// means reading old.txt, whose blob is not here.
	must(t, os.WriteFile(filepath.Join(repo, "new.txt"), []byte(body.String()+"one more line\n"), 0o644))
	plainGit(t, sb, repo, "update-index", "--add", "new.txt")
	plainGit(t, sb, repo, "update-index", "--force-remove", "old.txt")

	st, err := gitx.NewReader(sb.Git).Status(ctx, repo)
	if err != nil {
		t.Fatalf("status failed instead of retrying: %v", err)
	}
	if !st.RenamesSkipped {
		t.Fatalf("RenamesSkipped not set: %+v", st)
	}
	if st.Staged != 2 { // a deletion and an addition: the pair was not recognised as a rename
		t.Fatalf("staged = %d, want 2: %+v", st.Staged, st.Entries)
	}
}

// A repository that can be read normally never takes the retry path.
func TestStatusInPlainRepoKeepsRenames(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "plain", nil)
	sb.CommitFile(t, repo, "a.txt", "one\n")
	st, err := gitx.NewReader(sb.Git).Status(context.Background(), repo)
	if err != nil || st.RenamesSkipped {
		t.Fatalf("%+v %v", st, err)
	}
}

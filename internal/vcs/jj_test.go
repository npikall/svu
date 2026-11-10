package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJJDescribeTag(t *testing.T) {
	setup := func(tb testing.TB) {
		tb.Helper()
		tempdir(tb)
		jjInit(tb)
		jjCommit(tb, "chore: foobar")
		jjTag(tb, "pattern-1.2.3")
		jjCommit(tb, "lalalala")
		jjTag(tb, "v1.2.3")
		jjTag(tb, "v1.2.4") // multiple tags in a single commit
		jjCommit(tb, "chore: aaafoobar")
		jjCommit(tb, "docs: asdsad")
		jjCommit(tb, "fix: fooaaa")
		jjCommit(tb, "docs: update") // side branch starts here
		jjCommit(tb, "foo: bar")
		jjTag(tb, "v1.2.5")
		jjTag(tb, "v1.2.5-prerelease")
		jjCheckout(tb, `tags(exact:"v1.2.5")--`) // back to "fix: fooaaa"
	}
	t.Run(TagModeCurrent, func(t *testing.T) {
		setup(t)
		tag, err := Detect(".").DescribeTag(t.Context(), TagModeCurrent, "")
		require.NoError(t, err)
		require.Equal(t, "v1.2.4", tag)
	})

	t.Run(TagModeAll, func(t *testing.T) {
		setup(t)
		tag, err := Detect(".").DescribeTag(t.Context(), TagModeAll, "")
		require.NoError(t, err)
		require.Equal(t, "v1.2.5", tag)
	})

	t.Run("pattern", func(t *testing.T) {
		setup(t)
		tag, err := Detect(".").DescribeTag(t.Context(), TagModeCurrent, "pattern-*")
		require.NoError(t, err)
		require.Equal(t, "pattern-1.2.3", tag)
	})

	t.Run("no tags", func(t *testing.T) {
		tempdir(t)
		jjInit(t)
		jjCommit(t, "chore: foobar")
		tag, err := Detect(".").DescribeTag(t.Context(), TagModeAll, "")
		require.NoError(t, err)
		require.Empty(t, tag)
	})
}

func TestJJChangelog(t *testing.T) {
	tempdir(t)
	jjInit(t)
	jjCommit(t, "chore: foobar")
	jjCommit(t, "lalalala")
	jjTag(t, "v1.2.3")
	for _, msg := range []string{
		"chore: foobar",
		"fix: foo",
		"feat: foobar\n\nBREAKING CHANGE: body",
	} {
		jjCommit(t, msg)
	}
	log, err := Detect(".").Changelog(t.Context(), "v1.2.3", nil)
	require.NoError(t, err)
	requireLogContains(t, log, "chore: foobar")
	requireLogContains(t, log, "fix: foo")
	requireLogContains(t, log, "feat: foobar")
	requireLogNotContains(t, log, "lalalala")
	require.Contains(t, log[0].Body, "BREAKING CHANGE: body")
}

func TestJJChangelogWithoutTag(t *testing.T) {
	tempdir(t)
	jjInit(t)
	jjCommit(t, "chore: foobar")
	jjCommit(t, "fix: foo")
	log, err := Detect(".").Changelog(t.Context(), "", nil)
	require.NoError(t, err)
	require.Len(t, log, 2) // undescribed commits are not part of the history
	requireLogContains(t, log, "chore: foobar")
	requireLogContains(t, log, "fix: foo")
}

func TestJJChangelogWithDirectory(t *testing.T) {
	tempDir := tempdir(t)
	localDir := dir(t, tempDir)
	jjInit(t)
	jjCommit(t, "chore: foobar")
	jjCommit(t, "lalalala")
	jjTag(t, "v1.2.3")
	jjCommit(t, "feat: foobar")
	jjCommit(t, "chore: filtered dir")
	tempfile(t, localDir)
	log, err := Detect(".").Changelog(t.Context(), "v1.2.3", []string{localDir})
	require.NoError(t, err)

	requireLogContains(t, log, "chore: filtered dir")
	requireLogNotContains(t, log, "feat: foobar")
}

func TestJJRoot(t *testing.T) {
	tempDir := tempdir(t)
	jjInit(t)
	sub := dir(t, tempDir)
	root, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)
	require.Equal(t, root, Detect(sub).Root(t.Context()))
}

func TestDetect(t *testing.T) {
	t.Run("non-colocated jj", func(t *testing.T) {
		tempDir := tempdir(t)
		jjInit(t)
		require.IsType(t, jjBackend{}, Detect(dir(t, tempDir)).backend)
	})

	t.Run("colocated jj uses git", func(t *testing.T) {
		tempdir(t)
		_, err := fakeJJRun(t.Context(), "git", "init", "--colocate")
		require.NoError(t, err)
		require.IsType(t, gitBackend{}, Detect(".").backend)
	})

	t.Run("jj nested in git uses jj", func(t *testing.T) {
		tempDir := tempdir(t)
		gitInit(t)
		nested := dir(t, tempDir)
		t.Chdir(nested)
		jjInit(t)
		require.IsType(t, jjBackend{}, Detect(nested).backend)
	})

	t.Run("no repository falls back to git", func(t *testing.T) {
		require.IsType(t, gitBackend{}, Detect(t.TempDir()).backend)
	})
}

func TestSortTagsNewestFirst(t *testing.T) {
	tags := []string{
		"v1.9.0",
		"pattern-1.2.3",
		"v1.10.0-rc.1",
		"latest",
		"v1.10.0",
		"v1.2.3",
	}
	sortTagsNewestFirst(tags)
	require.Equal(t, []string{
		"v1.10.0",
		"v1.10.0-rc.1",
		"v1.9.0",
		"v1.2.3",
		"pattern-1.2.3",
		"latest",
	}, tags)
}

func jjInit(tb testing.TB) {
	tb.Helper()
	_, err := fakeJJRun(tb.Context(), "git", "init", "--no-colocate")
	require.NoError(tb, err)
}

// jjCommit starts a new commit on top of the working copy with msg.
func jjCommit(tb testing.TB, msg string) {
	tb.Helper()
	_, err := fakeJJRun(tb.Context(), "new", "-m", msg)
	require.NoError(tb, err)
}

func jjTag(tb testing.TB, tag string) {
	tb.Helper()
	_, err := fakeJJRun(tb.Context(), "tag", "set", tag, "-r", "@")
	require.NoError(tb, err)
}

func jjCheckout(tb testing.TB, revset string) {
	tb.Helper()
	_, err := fakeJJRun(tb.Context(), "new", revset)
	require.NoError(tb, err)
}

func fakeJJRun(ctx context.Context, args ...string) (string, error) {
	return jjRun(ctx, ".", append([]string{
		"--config", "user.name=svu",
		"--config", "user.email=svu@example.com",
		"--config", "signing.behavior=drop",
	}, args...)...)
}

func TestMain(m *testing.M) {
	// keep the developer's jj config out of the tests
	if err := os.Setenv("JJ_CONFIG", os.DevNull); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

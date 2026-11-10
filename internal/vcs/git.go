package vcs

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// gitBackend reads a git repository through the git CLI.
type gitBackend struct {
	dir string
}

// copied from goreleaser

func (b gitBackend) root(ctx context.Context) (string, error) {
	out, err := gitRun(ctx, b.dir, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func (b gitBackend) tags(ctx context.Context, tagMode string) ([]string, error) {
	args := []string{"-c", "versionsort.suffix=-", "tag", "--sort=-version:refname"}
	if tagMode == TagModeCurrent {
		args = append(args, "--merged")
	}
	tags, err := gitRun(ctx, b.dir, args...)
	if err != nil {
		return nil, err
	}
	return strings.Split(tags, "\n"), nil
}

func (b gitBackend) log(ctx context.Context, tag string, dirs []string) (string, error) {
	ref := "HEAD"
	if tag != "" {
		ref = fmt.Sprintf("tags/%s..HEAD", tag)
	}
	args := []string{"log", "--no-decorate", "--no-color", "--format=%H:%B" + commitEnd, ref}
	if len(dirs) > 0 {
		args = append(args, "--")
		args = append(args, dirs...)
	}
	return gitRun(ctx, b.dir, args...)
}

func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	extraArgs := []string{
		"-c", "log.showSignature=false",
	}
	args = append(extraArgs, args...)
	/* #nosec */
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	bts, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New(string(bts))
	}
	return string(bts), nil
}

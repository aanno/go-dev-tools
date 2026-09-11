package main

import (
	"fmt"
	"log"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// checkoutForSnapshot switches the worktree to rev (used when --to is given
// without --from in snapshot mode) and returns a restore func that switches
// back to whatever HEAD pointed to before, once called.
//
// It refuses to run on a dirty worktree, since switching commits would
// otherwise risk clobbering uncommitted changes.
func checkoutForSnapshot(repo *git.Repository, worktree *git.Worktree, rev string) (restore func(), err error) {
	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree status: %w", err)
	}
	if !status.IsClean() {
		return nil, fmt.Errorf("repo has uncommitted changes; snapshot mode with --to requires a clean worktree")
	}

	targetHash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve --to %q: %w", rev, err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get current HEAD: %w", err)
	}
	origBranch := head.Name()
	origHash := head.Hash()
	origIsBranch := origBranch.IsBranch()

	log.Printf("Checking out %s (%s) for snapshot analysis...", rev, targetHash)
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: *targetHash}); err != nil {
		return nil, fmt.Errorf("failed to checkout %s: %w", rev, err)
	}

	restore = func() {
		log.Printf("Restoring original HEAD...")
		opts := &git.CheckoutOptions{Hash: origHash}
		if origIsBranch {
			opts = &git.CheckoutOptions{Branch: origBranch}
		}
		if err := worktree.Checkout(opts); err != nil {
			log.Printf("WARNING: failed to restore original HEAD %s: %v (worktree left checked out at %s)", origHash, err, rev)
		}
	}
	return restore, nil
}

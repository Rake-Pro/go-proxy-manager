package store

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// DefaultFollowInterval is the poll period used by FollowRemote when none is
// given. Config changes are rare and operator-driven, so this only bounds how
// long a follower serves config the leader has already superseded.
const DefaultFollowInterval = 20 * time.Second

// PullFFOnly fast-forwards the config repo from its configured remote and
// reports whether HEAD moved. It never merges, rebases or resets: a diverged
// history is returned as an error for an operator to resolve. With no remote
// configured it is a no-op. Callers use the moved flag to reload only on a real
// change.
func (s *Store) PullFFOnly(ctx context.Context) (bool, error) {
	// The write lock: the pull rewrites the working tree, so it must not overlap
	// a Load or a commit.
	s.mu.Lock()
	defer s.mu.Unlock()
	before, err := s.git.Head(ctx)
	if err != nil {
		return false, err
	}
	if err := s.git.PullFFOnly(ctx); err != nil {
		return false, err
	}
	after, err := s.git.Head(ctx)
	if err != nil {
		return false, err
	}
	return after != before, nil
}

// FollowRemote is the HA follower's config poll loop: every interval it
// fast-forwards from the leader's repo and calls reload only when HEAD actually
// moved. It blocks until ctx is done; run it in a goroutine. interval <= 0 uses
// DefaultFollowInterval.
//
// Failures are logged and retried on the next tick rather than being fatal: a
// leader that is down or a diverged repo must not stop the follower serving the
// config it already has on disk.
func (s *Store) FollowRemote(ctx context.Context, interval time.Duration, reload func() error) {
	if interval <= 0 {
		interval = DefaultFollowInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	log.Info().Dur("interval", interval).Msg("HA follower: polling the leader config repo")
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			moved, err := s.PullFFOnly(ctx)
			if err != nil {
				log.Error().Err(err).Msg("HA follower: config pull failed; still serving the last synced config")
				continue
			}
			if !moved {
				continue
			}
			head, _ := s.Head(ctx)
			if reload == nil {
				continue
			}
			if err := reload(); err != nil {
				log.Error().Err(err).Str("head", head).Msg("HA follower: pulled new config but the reload failed")
				continue
			}
			log.Info().Str("head", head).Msg("HA follower: pulled and applied new config from the leader")
		}
	}
}

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

// PullTimeout bounds one fast-forward pull end to end. Without it a leader that
// accepts the TCP connection and then stalls (a hung git-http backend, a
// black-holed route, an SSH remote waiting on a prompt) leaves the pull running
// for as long as the peer keeps the socket open - and, before the lock split
// below, held the config write lock for exactly that long. The follower polls
// every DefaultFollowInterval, so a pull that has not completed inside this
// budget is simply retried on a later tick.
const PullTimeout = 60 * time.Second

// PullFFOnly fast-forwards the config repo from its configured remote and
// reports whether HEAD moved. It never merges, rebases or resets: a diverged
// history is returned as an error for an operator to resolve. With no remote
// configured it is a no-op. Callers use the moved flag to reload only on a real
// change.
//
// The network fetch runs WITHOUT the config write lock: it is the slow,
// remote-controlled part, and holding the lock across it stalls every config
// read and every admin write behind whatever the leader (or the network) decides
// to do. Only the local half - the HEAD compare and the fast-forward that
// rewrites the working tree - takes the lock, so a Load or a commit can never
// observe a half-applied tree. The whole operation is bounded by PullTimeout.
func (s *Store) PullFFOnly(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, PullTimeout)
	defer cancel()

	hasRemote, err := s.git.FetchRemote(ctx)
	if err != nil {
		return false, err
	}
	if !hasRemote {
		return false, nil // no remote configured; nothing to pull
	}

	// The write lock: the fast-forward rewrites the working tree, so it must not
	// overlap a Load or a commit.
	s.mu.Lock()
	defer s.mu.Unlock()
	before, err := s.git.Head(ctx)
	if err != nil {
		return false, err
	}
	if err := s.git.MergeFFOnly(ctx); err != nil {
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

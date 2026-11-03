// One-off data migration driven by the CLI (data:remove-salt). It rewrites
// every user row from the ECONUMO_DATA_SALT-encrypted form to plaintext so the
// salt can be removed from the environment afterwards. See CLAUDE.md.
package user

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MigrateRemoveDataSalt decrypts every user's email back to plaintext (AES
// ciphertext -> plaintext, decrypted with the salt the data was written with,
// passed in by the caller) so ECONUMO_DATA_SALT can be unset afterwards. The
// repo derives the identifier column from the row id at Save time, so this
// migration no longer touches it.
//
// The salt arrives as a parameter rather than via s.encode: the service's own
// encoder is salt-free (the API ignores ECONUMO_DATA_SALT), so this method builds
// the salted encoder it needs locally. The whole sweep runs in one transaction,
// so a mid-run failure rolls everything back — and because it locks each row
// before reading it, by the end it holds EVERY user's row lock until the commit:
// concurrent logins, resets and profile writes block behind it, so run it while
// writes are quiet.
//
// It is idempotent and mixed-state safe: a row whose email is already plaintext
// fails the salted Decode (bad base64 / HMAC mismatch) and is counted as
// "skipped" rather than corrupted, so a re-run reports migrated==0.
//
// Returns the number of rows rewritten and the number skipped (already
// plaintext / undecryptable).
func (s *Service) MigrateRemoveDataSalt(ctx context.Context, salt string) (migrated, skipped int, err error) {
	if strings.TrimSpace(salt) == "" {
		// With an empty salt Decode is a passthrough, so the sweep would store
		// ciphertext AS plaintext. Refuse rather than corrupt the data.
		return 0, 0, errors.New("data salt must be set to decrypt existing data")
	}
	ids, err := s.repo.ListIDs(ctx)
	if err != nil {
		return 0, 0, err
	}
	salted := auth.NewEncodeService(salt)

	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, id := range ids {
			// Same rule as every other whole-aggregate writer: lock, then read.
			if lerr := s.repo.LockRow(ctx, id); lerr != nil {
				return lerr
			}
			u, gerr := s.repo.GetByID(ctx, id)
			if gerr != nil {
				return gerr
			}
			plain, derr := salted.Decode(u.Email)
			if derr != nil {
				// Undecryptable under the current salt: already plaintext (e.g.
				// a re-run, or a row written before the salt was set). Leave it.
				skipped++
				continue
			}
			if plain == u.Email {
				// Nothing to change (defensive: Decode normally errors first).
				skipped++
				continue
			}
			u.UpdateEmail(plain, s.clock.Now())
			if serr := s.repo.Save(ctx, u); serr != nil {
				return serr
			}
			migrated++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return migrated, skipped, nil
}

// SeedEmailKeys fills users.email_key for every user that has none and returns how
// many it filled. The oldest account of a set whose addresses normalize alike takes
// the key. The others keep NULL, sign in by their exact spelling, and are logged for
// the operator to move with user:change-email: their data cannot be merged and
// failing the boot would take the instance down over them. Idempotent.
func (s *Service) SeedEmailKeys(ctx context.Context) (int, error) {
	var seeded int
	var collided []vo.Id
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, err := s.repo.ListWithoutEmailKey(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			key := model.NormalizeEmail(row.Email)
			taken, terr := s.repo.EmailKeyTaken(ctx, key)
			if terr != nil {
				return terr
			}
			if taken {
				collided = append(collided, row.ID)
				continue
			}
			if serr := s.repo.SetEmailKey(ctx, row.ID, key); serr != nil {
				return serr
			}
			seeded++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, id := range collided {
		slog.WarnContext(ctx, "user address collides with an older account, move it with user:change-email", "user_id", id.String())
	}
	return seeded, nil
}

// SeedAnalyticsOption writes the analytics preference row for every user that
// has none, using enabled (the deprecated ECONUMO_ANALYTICS value, read by the
// caller) as the seed so an operator who disabled analytics before the
// per-user preference existed does not silently start sending events again.
// Idempotent: users that already hold the option are skipped, so a rerun is a
// no-op.
//
// Writes ONLY the analytics option row via Repo.UpsertOption, not the whole
// user aggregate: this runs synchronously at boot, before the listener binds,
// over every user on the instance, so a GetByID+Save (whole user row plus
// every one of that user's other option rows) per user does not scale — on a
// large instance it can block startup for minutes. The whole sweep still runs
// in one transaction, so a mid-run failure rolls everything back and a retry
// converges (WHERE NOT EXISTS makes it safe to re-run either way).
func (s *Service) SeedAnalyticsOption(ctx context.Context, enabled bool) (int, error) {
	var seeded int
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		ids, err := s.repo.ListUserIDsMissingOption(ctx, model.OptionAnalytics)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		value := "0"
		if enabled {
			value = "1"
		}
		for _, id := range ids {
			opt := model.NewUserOption(s.repo.NextIdentity(), model.OptionAnalytics, &value, now)
			if serr := s.repo.UpsertOption(ctx, id, opt); serr != nil {
				return serr
			}
			seeded++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return seeded, nil
}

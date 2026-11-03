// One-off data migration driven by the CLI (data:remove-salt). It rewrites
// every user row from the ECONUMO_DATA_SALT-encrypted form to plaintext so the
// salt can be removed from the environment afterwards. See CLAUDE.md.
package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
// Decrypted addresses are stored normalized by the same rule as NormalizeEmails, so
// addresses that normalize alike do not fail the sweep. Addresses that are
// byte-identical do fail it, naming both accounts: no lookup could tell them apart.
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
	rows, err := s.repo.ListEmailsOldestFirst(ctx)
	if err != nil {
		return 0, 0, err
	}
	salted := auth.NewEncodeService(salt)

	var collided []vo.Id
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		var decrypted []StoredEmail
		for _, row := range rows {
			// Same rule as every other whole-aggregate writer: lock, then read.
			if lerr := s.repo.LockRow(ctx, row.ID); lerr != nil {
				return lerr
			}
			u, gerr := s.repo.GetByID(ctx, row.ID)
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
			decrypted = append(decrypted, StoredEmail{ID: row.ID, Email: plain})
		}
		// An address already in its normalized form has no other spelling to fall back
		// to, so those are placed first, whatever the accounts' ages.
		first := slices.DeleteFunc(slices.Clone(decrypted), func(r StoredEmail) bool { return r.Email != model.NormalizeEmail(r.Email) })
		rest := slices.DeleteFunc(slices.Clone(decrypted), func(r StoredEmail) bool { return r.Email == model.NormalizeEmail(r.Email) })
		for _, row := range slices.Concat(first, rest) {
			kept, perr := s.placeDecryptedEmail(ctx, row)
			if perr != nil {
				return perr
			}
			if kept {
				collided = append(collided, row.ID)
			}
			migrated++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	s.warnCollidingEmails(ctx, collided)
	return migrated, skipped, nil
}

// placeDecryptedEmail stores a decrypted address normalized, or as decrypted when that
// form is taken (reporting true). Two accounts with byte-identical addresses stop the
// sweep: no lookup could tell them apart, so the operator decides.
func (s *Service) placeDecryptedEmail(ctx context.Context, row StoredEmail) (bool, error) {
	normalized := model.NormalizeEmail(row.Email)
	holder, err := s.repo.HolderOfExactEmail(ctx, normalized)
	switch {
	case err != nil && !isNotFound(err):
		return false, err
	case err != nil:
		return false, s.repo.SetEmail(ctx, row.ID, normalized)
	case row.Email == normalized:
		return false, identicalEmailsError(holder, row.ID)
	}
	holder, err = s.repo.HolderOfExactEmail(ctx, row.Email)
	switch {
	case err == nil:
		return false, identicalEmailsError(holder, row.ID)
	case !isNotFound(err):
		return false, err
	}
	return true, s.repo.SetEmail(ctx, row.ID, row.Email)
}

func identicalEmailsError(a, b vo.Id) error {
	return fmt.Errorf("users %s and %s hold the same address: move one with user:change-email, then run this again", a, b)
}

// NormalizeEmails rewrites stored addresses to model.NormalizeEmail and returns how
// many it changed. An address whose normalized form another account already holds keeps
// its spelling, still signs in by it, and is logged for the operator to move with
// user:change-email: the data cannot be merged, and failing the boot would take the
// instance down. Addresses already normalized therefore keep their place, then the
// oldest account wins. Rows without an @ are still encrypted (base64 has none) and are
// left for data:remove-salt: lowercasing ciphertext would destroy it. Idempotent.
func (s *Service) NormalizeEmails(ctx context.Context) (int, error) {
	var changed int
	var collided []vo.Id
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, err := s.repo.ListEmailsOldestFirst(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			normalized := model.NormalizeEmail(row.Email)
			if normalized == row.Email || !strings.Contains(row.Email, "@") {
				continue
			}
			_, herr := s.repo.HolderOfExactEmail(ctx, normalized)
			switch {
			case herr == nil:
				collided = append(collided, row.ID)
				continue
			case !isNotFound(herr):
				return herr
			}
			if serr := s.repo.SetEmail(ctx, row.ID, normalized); serr != nil {
				return serr
			}
			changed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.warnCollidingEmails(ctx, collided)
	return changed, nil
}

func (s *Service) warnCollidingEmails(ctx context.Context, ids []vo.Id) {
	for _, id := range ids {
		slog.WarnContext(ctx, "user address collides with another account, move it with user:change-email", "user_id", id.String())
	}
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

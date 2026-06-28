// One-off data migration driven by the CLI (app:remove-data-salt). It rewrites
// every user row from the ECONUMO_DATA_SALT-encrypted form to plaintext so the
// salt can be removed from the environment afterwards. See CLAUDE.md.
package user

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/infra/auth"
)

// MigrateRemoveDataSalt decrypts every user's email back to plaintext and
// re-derives the identifier WITHOUT the data salt, so ECONUMO_DATA_SALT can be
// unset afterwards. It rewrites both salt-dependent columns:
//
//   - email:      AES ciphertext -> plaintext (s.encode.Decode, using the salt
//     the service was constructed with).
//   - identifier: hex(md5(lower(email)+salt)) -> hex(md5(lower(email))) — the
//     exact value Login will compute once the salt is unset.
//
// The caller MUST run this while the service's encoder still holds the salt the
// data was written with (the CLI command refuses an empty salt). The whole sweep
// runs in one transaction, so a mid-run failure rolls everything back.
//
// It is idempotent and mixed-state safe: a row whose email is already plaintext
// fails the salted Decode (bad base64 / HMAC mismatch) and is counted as
// "skipped" rather than corrupted, so a re-run reports migrated==0.
//
// Returns the number of rows rewritten and the number skipped (already
// plaintext / undecryptable).
func (s *Service) MigrateRemoveDataSalt(ctx context.Context) (migrated, skipped int, err error) {
	ids, err := s.repo.ListIDs(ctx)
	if err != nil {
		return 0, 0, err
	}
	// Salt-free hasher: derives the post-removal identifier md5(lower(email)).
	saltFree := auth.NewEncodeService("")

	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, id := range ids {
			u, gerr := s.repo.GetByID(ctx, id)
			if gerr != nil {
				return gerr
			}
			plain, derr := s.encode.Decode(u.Email())
			if derr != nil {
				// Undecryptable under the current salt: already plaintext (e.g.
				// a re-run, or a row written before the salt was set). Leave it.
				skipped++
				continue
			}
			newIdent := saltFree.Hash(strings.ToLower(plain))
			if plain == u.Email() && newIdent == u.Identifier() {
				// Nothing to change (defensive: Decode normally errors first).
				skipped++
				continue
			}
			// Avatar is the gravatar of the email's md5 — independent of the data
			// salt — so the stored value is already correct; pass it through.
			u.UpdateEmail(newIdent, plain, u.AvatarURL(), s.clock.Now())
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

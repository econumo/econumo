ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT '0' NOT NULL;
-- Existing users predate verification, so grandfather them in as verified; the
-- column default stays false so any future insert that forgets the field fails
-- closed (unverified) rather than open.
UPDATE users SET email_verified = '1';

CREATE TABLE users_email_verifications
(
    id         TEXT NOT NULL
    , user_id    TEXT NOT NULL
    , code       TEXT NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , expired_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);

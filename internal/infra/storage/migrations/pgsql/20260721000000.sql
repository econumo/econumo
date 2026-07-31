ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT false NOT NULL;
-- Existing users predate verification, so grandfather them in as verified; the
-- column default stays false so any future insert that forgets the field fails
-- closed (unverified) rather than open.
UPDATE users SET email_verified = true;

CREATE TABLE users_email_verifications
(
    id         UUID NOT NULL
    , user_id    UUID NOT NULL
    , code       VARCHAR(64) NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expired_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);

ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT true NOT NULL;

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

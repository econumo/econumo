CREATE TABLE users_email_change_requests
(
    id         UUID NOT NULL
    , user_id    UUID NOT NULL
    , new_email  VARCHAR(255) NOT NULL
    , code       VARCHAR(64) NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expired_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);

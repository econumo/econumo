CREATE TABLE users_email_change_requests
(
    id         TEXT NOT NULL
    , user_id    TEXT NOT NULL
    , new_email  VARCHAR(255) NOT NULL
    , code       TEXT NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , expired_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);

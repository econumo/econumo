CREATE TABLE users
(
    id         CHAR(36)       NOT NULL
    , identifier CHAR(32)     NOT NULL
    , email      VARCHAR(255) NOT NULL
    , name       VARCHAR(255) NOT NULL
    , avatar_url VARCHAR(255) NOT NULL
    , password   VARCHAR(255) NOT NULL
    , salt       VARCHAR(40)  NOT NULL
    , created_at DATETIME     NOT NULL
    , updated_at DATETIME     NOT NULL
    , PRIMARY KEY (id)
    , UNIQUE (identifier)
);
CREATE UNIQUE INDEX UNIQ_1483A5E9772E836A ON users (identifier);
CREATE TABLE currencies
(
    id         CHAR(36)    NOT NULL
    , code       CHAR(3)     NOT NULL
    , symbol     VARCHAR(12) NOT NULL
    , created_at DATETIME    NOT NULL
    , PRIMARY KEY (id)
    , UNIQUE (code)
);
CREATE UNIQUE INDEX UNIQ_37C4469377153098 ON currencies (code);
CREATE TABLE accounts
(
    id                      CHAR(36)            NOT NULL
    , currency_id             CHAR(36)            NOT NULL
    , user_id                 CHAR(36)            NOT NULL
    , name                    VARCHAR(64)         NOT NULL
    , type                    SMALLINT            NOT NULL
    , icon                    VARCHAR(64)         NOT NULL
    , is_deleted              BOOLEAN DEFAULT '0' NOT NULL
    , created_at              DATETIME            NOT NULL
    , updated_at              DATETIME            NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_CAC89EAC38248176 ON accounts (currency_id);
CREATE INDEX IDX_CAC89EACA76ED395 ON accounts (user_id);
CREATE INDEX user_id_is_deleted_idx_accounts ON accounts (user_id, is_deleted);
CREATE INDEX is_deleted_idx_accounts ON accounts (is_deleted);
CREATE TABLE accounts_access
(
    account_id CHAR(36) NOT NULL
    , user_id    CHAR(36) NOT NULL
    , role       SMALLINT NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , PRIMARY KEY (account_id, user_id)
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_98A8AF869B6B5FBA ON accounts_access (account_id);
CREATE INDEX IDX_98A8AF86A76ED395 ON accounts_access (user_id);
CREATE TABLE accounts_options
(
    account_id CHAR(36)                    NOT NULL
    , user_id    CHAR(36)                    NOT NULL
    , position   SMALLINT UNSIGNED DEFAULT 0 NOT NULL
    , created_at DATETIME                    NOT NULL
    , updated_at DATETIME                    NOT NULL
    , PRIMARY KEY (account_id, user_id)
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_B87688FB9B6B5FBA ON accounts_options (account_id);
CREATE INDEX IDX_B87688FBA76ED395 ON accounts_options (user_id);
CREATE TABLE users_connections
(
    user_id           CHAR(36) NOT NULL
    , connected_user_id CHAR(36) NOT NULL
    , PRIMARY KEY (user_id, connected_user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (connected_user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_4843C0E7A76ED395 ON users_connections (user_id);
CREATE INDEX IDX_4843C0E7349E946C ON users_connections (connected_user_id);
CREATE TABLE users_connections_invites
(
    user_id    CHAR(36) NOT NULL
    , code       VARCHAR(255) DEFAULT NULL
    , expired_at DATETIME     DEFAULT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
);
CREATE INDEX expired_at_idx_connections_invites ON users_connections_invites (expired_at);
CREATE INDEX user_id_idx_connections_invites ON users_connections_invites (user_id);
CREATE UNIQUE INDEX code_uniq_connections_invites ON users_connections_invites (code);
CREATE TABLE folders
(
    id         CHAR(36)                         NOT NULL
    , user_id    CHAR(36)                         NOT NULL
    , name       VARCHAR(64)                      NOT NULL
    , position   SMALLINT UNSIGNED DEFAULT 0      NOT NULL
    , is_visible BOOLEAN           DEFAULT 'true' NOT NULL
    , created_at DATETIME                         NOT NULL
    , updated_at DATETIME                         NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_FE37D30FA76ED395 ON folders (user_id);
CREATE TABLE accounts_folders
(
    folder_id  CHAR(36) NOT NULL
    , account_id CHAR(36) NOT NULL
    , PRIMARY KEY (folder_id, account_id)
    , FOREIGN KEY (folder_id) REFERENCES folders (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX IDX_9674A173162CB942 ON accounts_folders (folder_id);
CREATE INDEX IDX_9674A1739B6B5FBA ON accounts_folders (account_id);
CREATE TABLE payees
(
    id          CHAR(36)                      NOT NULL
    , user_id     CHAR(36)                      NOT NULL
    , name        VARCHAR(64)                   NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0   NOT NULL
    , is_archived BOOLEAN           DEFAULT '0' NOT NULL
    , created_at  DATETIME                      NOT NULL
    , updated_at  DATETIME                      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_971FAB26A76ED395 ON payees (user_id);
CREATE TABLE tags
(
    id          CHAR(36)                      NOT NULL
    , user_id     CHAR(36)                      NOT NULL
    , name        VARCHAR(64)                   NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0   NOT NULL
    , is_archived BOOLEAN           DEFAULT '0' NOT NULL
    , created_at  DATETIME                      NOT NULL
    , updated_at  DATETIME                      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_6FBC9426A76ED395 ON tags (user_id);
CREATE TABLE categories
(
    id          CHAR(36)                      NOT NULL
    , user_id     CHAR(36)                      NOT NULL
    , name        VARCHAR(64)                   NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0   NOT NULL
    , type        SMALLINT                      NOT NULL
    , icon        VARCHAR(255)                  NOT NULL
    , is_archived BOOLEAN           DEFAULT '0' NOT NULL
    , created_at  DATETIME                      NOT NULL
    , updated_at  DATETIME                      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_3AF34668A76ED395 ON categories (user_id);
CREATE TABLE transactions
(
    id                   CHAR(36)       NOT NULL
    , user_id              CHAR(36)       NOT NULL
    , account_id           CHAR(36)       NOT NULL
    , account_recipient_id CHAR(36)       DEFAULT NULL
    , category_id          CHAR(36)       DEFAULT NULL
    , payee_id             CHAR(36)       DEFAULT NULL
    , tag_id               CHAR(36)       DEFAULT NULL
    , type                 SMALLINT       NOT NULL
    , amount               NUMERIC(19, 2) NOT NULL
    , amount_recipient     NUMERIC(19, 2) DEFAULT NULL
    , description          VARCHAR(255)   NOT NULL
    , created_at           DATETIME       NOT NULL
    , updated_at           DATETIME       NOT NULL
    , spent_at             DATETIME       NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (account_recipient_id) REFERENCES accounts (id) ON DELETE SET NULL
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL
    , FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL
    , FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL
);
CREATE INDEX IDX_EAA81A4CA76ED395 ON transactions (user_id);
CREATE INDEX IDX_EAA81A4C9B6B5FBA ON transactions (account_id);
CREATE INDEX IDX_EAA81A4C70F7993E ON transactions (account_recipient_id);
CREATE INDEX IDX_EAA81A4C12469DE2 ON transactions (category_id);
CREATE INDEX IDX_EAA81A4CCB4B68F ON transactions (payee_id);
CREATE INDEX IDX_EAA81A4CBAD26311 ON transactions (tag_id);
CREATE INDEX type_idx_transactions ON transactions (type);
CREATE INDEX spent_idx_transactions ON transactions (spent_at);
CREATE INDEX account_id_spent_at_idx_transactions ON transactions (account_id, spent_at);
CREATE INDEX account_recipient_id_spent_at_idx_transactions ON transactions (account_recipient_id, spent_at);
CREATE INDEX category_id_account_id_spent_at_idx_transactions ON transactions (category_id, account_id, spent_at);
CREATE INDEX tag_id_account_id_spent_at_idx_transactions ON transactions (tag_id, account_id, spent_at);
CREATE TABLE users_options
(
    id         CHAR(36)     NOT NULL
    , user_id    CHAR(36)     NOT NULL
    , name       VARCHAR(255) NOT NULL
    , value      VARCHAR(256) DEFAULT NULL
    , created_at DATETIME     NOT NULL
    , updated_at DATETIME     NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (user_id, name)
);
CREATE INDEX IDX_20358E4DA76ED395 ON users_options (user_id);
CREATE UNIQUE INDEX identifier_uniq_users_options ON users_options (user_id, name);
CREATE TABLE users_password_requests
(
    id         CHAR(36) NOT NULL
    , user_id    CHAR(36) NOT NULL
    , code       CHAR(12) NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , expired_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);
CREATE UNIQUE INDEX UNIQ_4DBE72F977153098 ON users_password_requests (code);
CREATE UNIQUE INDEX UNIQ_4DBE72F9A76ED395 ON users_password_requests (user_id);
CREATE TABLE currencies_rates
(
    id               CHAR(36)       NOT NULL
    , currency_id      CHAR(36)       NOT NULL
    , base_currency_id CHAR(36)       NOT NULL
    , rate             NUMERIC(16, 8) NOT NULL
    , published_at     DATE           NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
    , FOREIGN KEY (base_currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE INDEX IDX_5AA604E038248176 ON currencies_rates (currency_id);
CREATE INDEX IDX_5AA604E03101778E ON currencies_rates (base_currency_id);
CREATE INDEX published_at_idx_currencies_rates ON currencies_rates (published_at);
CREATE INDEX currency_id_published_at_idx_currencies_rates ON currencies_rates (currency_id, published_at);
CREATE INDEX base_currency_id_published_at_idx_currencies_rates ON currencies_rates (base_currency_id, published_at);
CREATE UNIQUE INDEX identifier_uniq_currencies_rates ON currencies_rates (published_at, currency_id, base_currency_id);
CREATE TABLE operation_requests_ids
(
    id         CHAR(36)            NOT NULL
    , is_handled BOOLEAN DEFAULT '0' NOT NULL
    , created_at DATETIME            NOT NULL
    , updated_at DATETIME            NOT NULL
    , PRIMARY KEY (id)
);
CREATE TABLE messenger_messages
(
    id           INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL
    , body         CLOB                              NOT NULL
    , headers      CLOB                              NOT NULL
    , queue_name   VARCHAR(190)                      NOT NULL
    , created_at   DATETIME                          NOT NULL
    , available_at DATETIME                          NOT NULL
    , delivered_at DATETIME DEFAULT NULL
);
CREATE INDEX IDX_75EA56E0FB7336F0 ON messenger_messages (queue_name);
CREATE INDEX IDX_75EA56E0E3BD61CE ON messenger_messages (available_at);
CREATE INDEX IDX_75EA56E016BA31DB ON messenger_messages (delivered_at);
INSERT INTO currencies (id, code, symbol, created_at) VALUES ('dffc2a06-6f29-4704-8575-31709adee926', 'USD', '$', '2021-08-12 21:05:48');

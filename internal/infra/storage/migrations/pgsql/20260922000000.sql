-- See the sqlite sibling for the semantics.
CREATE TABLE budgets_elements_comments
(
    id           UUID     NOT NULL
    , element_id UUID     NOT NULL
    , period     TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , user_id    UUID     NOT NULL
    , comment    TEXT     NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX budgets_elements_comments_element_period_idx ON budgets_elements_comments (element_id, period);
CREATE INDEX budgets_elements_comments_period_idx ON budgets_elements_comments (period);

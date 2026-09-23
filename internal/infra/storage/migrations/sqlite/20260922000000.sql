-- Budget cell comments: a flat thread per (budget element, month).
-- element_id cascades, so removing an element or a whole budget takes its
-- threads with it. period is the first of the month, stored as bare
-- 'Y-m-d H:i:s' text like every other datetime column.
CREATE TABLE budgets_elements_comments
(
    id           TEXT     NOT NULL
    , element_id TEXT     NOT NULL
    , period     DATETIME NOT NULL
    , user_id    TEXT     NOT NULL
    , comment    TEXT     NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX budgets_elements_comments_element_period_idx ON budgets_elements_comments (element_id, period);
CREATE INDEX budgets_elements_comments_period_idx ON budgets_elements_comments (period);

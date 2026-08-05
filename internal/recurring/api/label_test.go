package api_test

// labelIds coverage on the recurring endpoints: a genuine round trip on
// create/read, full-replace clear semantics on update, transfer's "always
// empty" rule, dedupe, the two rejection paths (unknown id, foreign-owned
// id), the shared-account owner-label rule, and — the core of this task —
// that posting a due template copies its labels onto the created
// transaction, and that a locked replay does not duplicate that attachment.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const recurringAcct2ID = "aaaa1111-0000-0000-0000-0000000000a2"

// seedSecondAccount adds a second seed-user account, for transfer templates.
func (h *harness) seedSecondAccount(t *testing.T) {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.Account(fixture.Account{ID: recurringAcct2ID, UserID: seedUserID, CurrencyID: usdID, Name: "Bank"})
	f.AccountInFolder(folderID, recurringAcct2ID)
	f.AccountOption(recurringAcct2ID, seedUserID, 1)
}

func createRecurringReqWithLabels(opID, typ, amount string, labelIDs []string) map[string]any {
	req := createRecurringReq(opID, typ, amount)
	req["labelIds"] = labelIDs
	return req
}

func transferRecurringReq(opID, amount string) map[string]any {
	return map[string]any{
		"id": opID, "type": "transfer", "amount": amount,
		"accountId": accountID, "accountRecipientId": recurringAcct2ID,
		"schedule": "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
	}
}

type recurringItemResult struct {
	Item recurringItem `json:"item"`
}

// TestCreateRecurringTransaction_WithLabels_RoundTrip: the create response
// carries both ids in ascending-string order even though the request lists
// them reversed, and a subsequent get-recurring-transaction-list read of the
// PERSISTED row shows the same two ids in the same order.
func TestCreateRecurringTransaction_WithLabels_RoundTrip(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opID = "0197c400-0000-7000-8000-000000000001"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels(opID, "expense", "10", []string{label2ID, label1ID}))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":["`+label1ID+`","`+label2ID+`"]`) {
		t.Fatalf("raw body missing sorted labelIds array; body: %s", env.raw)
	}
	res := mustUnmarshal[recurringItemResult](t, env.Data)
	if len(res.Item.LabelIds) != 2 || res.Item.LabelIds[0] != label1ID || res.Item.LabelIds[1] != label2ID {
		t.Fatalf("labelIds = %v, want [%s %s] (sorted)", res.Item.LabelIds, label1ID, label2ID)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	list := mustUnmarshal[recurringList](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v, want one item", list.Items)
	}
	if got := list.Items[0].LabelIds; len(got) != 2 || got[0] != label1ID || got[1] != label2ID {
		t.Fatalf("persisted list labelIds = %v, want [%s %s]", got, label1ID, label2ID)
	}
}

// TestCreateRecurringTransaction_DuplicateLabelIds_Deduped: the same id
// repeated in the request must land exactly once — without dedup, the second
// insert into recurring_transactions_labels would violate its composite PK.
func TestCreateRecurringTransaction_DuplicateLabelIds_Deduped(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels("0197c400-0000-7000-8000-000000000002", "expense", "10", []string{label1ID, label1ID, label2ID}))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[recurringItemResult](t, env.Data)
	if len(res.Item.LabelIds) != 2 {
		t.Fatalf("labelIds = %v, want exactly 2 (deduped)", res.Item.LabelIds)
	}
}

// TestCreateRecurringTransaction_UnknownLabel_Rejected: a well-formed but
// non-existent label id is rejected, same as an unavailable category/tag.
func TestCreateRecurringTransaction_UnknownLabel_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels("0197c400-0000-7000-8000-000000000003", "expense", "10", []string{"dddd4444-0000-0000-0000-0000000000ff"}))
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}

// TestCreateRecurringTransaction_ForeignLabel_Rejected: a label owned by a
// user who shares no account with the caller must not be attachable to a
// template — labelIds is not a way to probe or borrow another user's
// classification.
func TestCreateRecurringTransaction_ForeignLabel_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels("0197c400-0000-7000-8000-000000000004", "expense", "10", []string{otherLabelID}))
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}

// TestCreateRecurringTransaction_SharedAccount_UsesOwnerLabel_Succeeds: on a
// shared account, the caller classifies a template with the ACCOUNT OWNER's
// label, not their own — a label must belong to the account owner
// specifically, matching how shared-account category/payee/tag already work
// for transactions.
func TestCreateRecurringTransaction_SharedAccount_UsesOwnerLabel_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 1, true) // role user (admin=0, user=1, guest=2): write + own-label use
	const ownerLabelID = "cccc4444-0000-0000-0000-00000000c2cc"
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.Label(fixture.Label{ID: ownerLabelID, UserID: recOwnerTwoID, Name: "Owner Label"})

	tok := h.token(t)
	body := sharedCreateRecurringReq("0197c400-0000-7000-8000-000000000009", "10")
	body["labelIds"] = []string{ownerLabelID}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[recurringItemResult](t, env.Data)
	if len(res.Item.LabelIds) != 1 || res.Item.LabelIds[0] != ownerLabelID {
		t.Fatalf("labelIds = %v, want [%s]", res.Item.LabelIds, ownerLabelID)
	}
}

// TestCreateRecurringTransaction_TransferLabelIds_Ignored: a transfer
// template never carries labels; the request's labelIds is silently dropped
// rather than rejected, and the response shows an empty (not null) list.
func TestCreateRecurringTransaction_TransferLabelIds_Ignored(t *testing.T) {
	h := newHarness(t)
	h.seedSecondAccount(t)
	tok := h.token(t)
	body := transferRecurringReq("0197c400-0000-7000-8000-000000000005", "10")
	body["labelIds"] = []string{label1ID, label2ID}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw body labelIds not empty for a transfer template; body: %s", env.raw)
	}
	res := mustUnmarshal[recurringItemResult](t, env.Data)
	if res.Item.LabelIds == nil || len(res.Item.LabelIds) != 0 {
		t.Fatalf("transfer template labelIds = %v, want [] (ignored)", res.Item.LabelIds)
	}
}

// TestUpdateRecurringTransaction_LabelIds_Clears: update-recurring-transaction
// is a full-state replace (same rule as categoryId), so an explicit
// "labelIds": [] clears a previously-set label list, and the clear persists
// (checked via the list endpoint, not just the update response).
func TestUpdateRecurringTransaction_LabelIds_Clears(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels("0197c400-0000-7000-8000-000000000006", "expense", "10", []string{label1ID, label2ID}))
	created := mustUnmarshal[recurringItemResult](t, cEnv.Data).Item
	if len(created.LabelIds) != 2 {
		t.Fatalf("labelIds before clearing = %v, want 2", created.LabelIds)
	}

	body := map[string]any{
		"id": created.ID, "type": "expense", "amount": "10",
		"accountId": accountID, "schedule": "weekly",
		"nextPaymentAt": "2026-09-05 00:00:00", "labelIds": []string{},
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/update-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw body labelIds not an empty array; body: %s", env.raw)
	}
	res := mustUnmarshal[recurringItemResult](t, env.Data)
	if res.Item.LabelIds == nil || len(res.Item.LabelIds) != 0 {
		t.Fatalf("labelIds after clearing = %v, want [] (non-nil, empty)", res.Item.LabelIds)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	list := mustUnmarshal[recurringList](t, listEnv.Data)
	if len(list.Items) != 1 || len(list.Items[0].LabelIds) != 0 {
		t.Fatalf("persisted list labelIds = %+v, want one item with none", list.Items)
	}
}

type transactionItemWithLabels struct {
	ID       string   `json:"id"`
	LabelIds []string `json:"labelIds"`
}

// TestPostRecurringCopiesLabels: posting a due template copies its labels
// onto the created transaction — visible both in the post response and in a
// subsequent read of the persisted transaction (proving the join-table write
// actually happened, not just that the in-memory response looked right).
func TestPostRecurringCopiesLabels(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	cStatus, cEnv := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels("0197c400-0000-7000-8000-000000000007", "expense", "42.50", []string{label2ID, label1ID}))
	if cStatus != http.StatusOK {
		t.Fatalf("create status=%d body=%s", cStatus, cEnv.raw)
	}
	item := mustUnmarshal[recurringItemResult](t, cEnv.Data).Item

	const txOpID = "0197c500-0000-7000-8000-000000000001"
	body := map[string]any{
		"recurringId": item.ID, "id": txOpID, "type": "expense", "amount": "42.50",
		"accountId": accountID, "categoryId": catID, "date": "2026-08-31 00:00:00", "description": "rent",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("post status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":["`+label1ID+`","`+label2ID+`"]`) {
		t.Fatalf("raw post response missing copied sorted labelIds; body: %s", env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	if len(res.Item.LabelIds) != 2 || res.Item.LabelIds[0] != label1ID || res.Item.LabelIds[1] != label2ID {
		t.Fatalf("post response labelIds = %v, want [%s %s]", res.Item.LabelIds, label1ID, label2ID)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[struct {
		Items []transactionItemWithLabels `json:"items"`
	}](t, listEnv.Data)
	found := false
	for _, it := range list.Items {
		if it.ID == res.Item.ID {
			found = true
			if len(it.LabelIds) != 2 || it.LabelIds[0] != label1ID || it.LabelIds[1] != label2ID {
				t.Fatalf("persisted transaction labelIds = %v, want [%s %s]", it.LabelIds, label1ID, label2ID)
			}
		}
	}
	if !found {
		t.Fatalf("posted transaction %q not found in get-transaction-list", res.Item.ID)
	}
}

// TestPostRecurringTransferTemplate_LabelsEmpty: a transfer template carries
// no labels (normalize() already cleared them at create time), so posting
// one yields a transaction with labelIds: [].
func TestPostRecurringTransferTemplate_LabelsEmpty(t *testing.T) {
	h := newHarness(t)
	h.seedSecondAccount(t)
	tok := h.token(t)
	cStatus, cEnv := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		transferRecurringReq("0197c400-0000-7000-8000-000000000008", "10"))
	if cStatus != http.StatusOK {
		t.Fatalf("create status=%d body=%s", cStatus, cEnv.raw)
	}
	item := mustUnmarshal[recurringItemResult](t, cEnv.Data).Item

	body := map[string]any{
		"recurringId": item.ID, "id": "0197c500-0000-7000-8000-000000000002", "type": "transfer",
		"amount": "10", "accountId": accountID, "accountRecipientId": recurringAcct2ID,
		"date": "2026-08-31 00:00:00",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("post status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw post response labelIds not empty for a transfer; body: %s", env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	if res.Item.LabelIds == nil || len(res.Item.LabelIds) != 0 {
		t.Fatalf("transfer post labelIds = %v, want []", res.Item.LabelIds)
	}
}

// TestPostRecurringTwiceDoesNotDuplicateLabels: posting is idempotent on the
// client-supplied id — a replay with the SAME id is rejected by the
// operation guard before any further write runs. This proves the replay
// leaves the already-attached labels exactly as they were: not cleared, and
// critically not duplicated (checked directly against
// transactions_labels, not just the — unaffected — response body).
func TestPostRecurringTwiceDoesNotDuplicateLabels(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	cStatus, cEnv := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels("0197c400-0000-7000-8000-00000000000a", "expense", "10", []string{label1ID}))
	if cStatus != http.StatusOK {
		t.Fatalf("create status=%d body=%s", cStatus, cEnv.raw)
	}
	item := mustUnmarshal[recurringItemResult](t, cEnv.Data).Item

	const txOpID = "0197c500-0000-7000-8000-000000000003"
	body := map[string]any{
		"recurringId": item.ID, "id": txOpID, "type": "expense", "amount": "10",
		"accountId": accountID, "categoryId": catID, "date": "2026-08-31 00:00:00",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("first post status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	if len(res.Item.LabelIds) != 1 || res.Item.LabelIds[0] != label1ID {
		t.Fatalf("first post labelIds = %v, want [%s]", res.Item.LabelIds, label1ID)
	}

	status2, env2 := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status2 != http.StatusBadRequest {
		t.Fatalf("replay status=%d, want 400 (operation locked); body: %s", status2, env2.raw)
	}

	var count int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM transactions_labels WHERE transaction_id = ?", res.Item.ID).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("transactions_labels rows for %q = %d, want 1 (no duplication from the locked replay)", res.Item.ID, count)
	}
}

// maxTestRecurringLabels mirrors internal/recurring/usecase.go's unexported
// maxRecurringLabels (50) -- this test package cannot see it directly, so
// the boundary is duplicated here; keep the two in sync.
const maxTestRecurringLabels = 50

// TestCreateRecurringTransaction_TooManyLabels_Rejected: one over the cap is
// rejected with the dedicated too-many-labels error BEFORE any per-id
// ownership lookup runs -- none of these ids need to exist for the cap to
// fire, since resolveLabels checks the deduped count first. The cap runs
// ahead of LabelOwners, which is a per-id GetByID loop, so this also guards
// against turning a bad request into thousands of sequential lookups inside
// an open write transaction (see internal/transaction/api/label_test.go's
// TestCreateTransaction_TooManyLabels_Rejected, the sibling this mirrors).
func TestCreateRecurringTransaction_TooManyLabels_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ids := make([]string, maxTestRecurringLabels+1)
	for i := range ids {
		ids[i] = fixture.NewID()
	}
	const opID = "0197c400-0000-7000-8000-000000000008"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels(opID, "expense", "12.00", ids))
	assertValidationDenied(t, status, env, "A transaction can have at most 50 labels.")
}

// TestCreateRecurringTransaction_MaxLabels_Accepted: exactly the cap
// succeeds, proving the check is an off-by-one-safe ">", not ">=".
func TestCreateRecurringTransaction_MaxLabels_Accepted(t *testing.T) {
	h := newHarness(t)
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	ids := make([]string, maxTestRecurringLabels)
	for i := range ids {
		ids[i] = f.Label(fixture.Label{UserID: seedUserID, Name: fixture.NewID()})
	}

	tok := h.token(t)
	const opID = "0197c400-0000-7000-8000-000000000009"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels(opID, "expense", "12.00", ids))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[recurringItemResult](t, env.Data)
	if len(res.Item.LabelIds) != maxTestRecurringLabels {
		t.Fatalf("labelIds count = %d, want %d", len(res.Item.LabelIds), maxTestRecurringLabels)
	}
}

// postLabelsFixture creates a template carrying label1+label2 and returns its
// id, for the three post-time labelIds cases below.
func postLabelsFixture(t *testing.T, h *harness, tok, opID string) string {
	t.Helper()
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels(opID, "expense", "42.50", []string{label1ID, label2ID}))
	if status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", status, env.raw)
	}
	return mustUnmarshal[recurringItemResult](t, env.Data).Item.ID
}

func postBody(rtID, opID string) map[string]any {
	return map[string]any{
		"recurringId": rtID, "id": opID, "type": "expense", "amount": "42.50",
		"accountId": accountID, "categoryId": catID, "date": "2026-08-31 00:00:00", "description": "rent",
	}
}

// persistedTxLabels reads the labels actually stored for a transaction, so
// these tests prove the join-table write rather than a well-formed response.
func persistedTxLabels(t *testing.T, h *harness, tok, txID string) []string {
	t.Helper()
	_, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[struct {
		Items []transactionItemWithLabels `json:"items"`
	}](t, env.Data)
	for _, it := range list.Items {
		if it.ID == txID {
			return it.LabelIds
		}
	}
	t.Fatalf("transaction %q not found in get-transaction-list", txID)
	return nil
}

// templateLabels reads a template's own labels, to prove a post-time override
// changes only the created transaction and never the template itself.
func templateLabels(t *testing.T, h *harness, tok, rtID string) []string {
	t.Helper()
	_, env := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	for _, it := range mustUnmarshal[recurringList](t, env.Data).Items {
		if it.ID == rtID {
			return it.LabelIds
		}
	}
	t.Fatalf("template %q not found in get-recurring-transaction-list", rtID)
	return nil
}

// TestPostRecurring_AbsentLabelIds_InheritsTemplate: omitting labelIds keeps
// the pre-existing contract — the template's labels are copied on. This is
// the case every client written before the field existed relies on, so it
// must stay byte-for-byte what TestPostRecurringCopiesLabels already asserts;
// this one states the ABSENT half of the three-case rule explicitly.
func TestPostRecurring_AbsentLabelIds_InheritsTemplate(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	rtID := postLabelsFixture(t, h, tok, "0197c400-0000-7000-8000-00000000000b")

	body := postBody(rtID, "0197c500-0000-7000-8000-000000000010")
	if _, ok := body["labelIds"]; ok {
		t.Fatal("this case is about labelIds being ABSENT from the body")
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("post status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	want := []string{label1ID, label2ID}
	if len(res.Item.LabelIds) != 2 || res.Item.LabelIds[0] != want[0] || res.Item.LabelIds[1] != want[1] {
		t.Fatalf("post response labelIds = %v, want %v (inherited)", res.Item.LabelIds, want)
	}
	if got := persistedTxLabels(t, h, tok, res.Item.ID); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("persisted labelIds = %v, want %v", got, want)
	}
}

// TestPostRecurring_ExplicitLabelIds_ReplacesTemplateSet: an explicit list is
// the caller's own set — the user toggled the chips at post time — and the
// template's labels are NOT copied over it. The template itself is unchanged.
func TestPostRecurring_ExplicitLabelIds_ReplacesTemplateSet(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	rtID := postLabelsFixture(t, h, tok, "0197c400-0000-7000-8000-00000000000c")

	body := postBody(rtID, "0197c500-0000-7000-8000-000000000011")
	body["labelIds"] = []string{label2ID}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("post status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	if len(res.Item.LabelIds) != 1 || res.Item.LabelIds[0] != label2ID {
		t.Fatalf("post response labelIds = %v, want [%s] (the request's own set)", res.Item.LabelIds, label2ID)
	}
	if got := persistedTxLabels(t, h, tok, res.Item.ID); len(got) != 1 || got[0] != label2ID {
		t.Fatalf("persisted labelIds = %v, want [%s]", got, label2ID)
	}
	if got := templateLabels(t, h, tok, rtID); len(got) != 2 {
		t.Fatalf("template labelIds = %v, want both still attached (a post never edits the template)", got)
	}
}

// TestPostRecurring_ExplicitEmptyLabelIds_PostsWithNone: the sharp edge —
// "labelIds": [] means NO labels, not "inherit". Getting this wrong turns
// every deliberate clear back into a copy of the template's set (the mirror
// image of the MCP update_transaction bug that UpdateTransactionPreservingLabels
// exists to undo).
func TestPostRecurring_ExplicitEmptyLabelIds_PostsWithNone(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	rtID := postLabelsFixture(t, h, tok, "0197c400-0000-7000-8000-00000000000d")

	body := postBody(rtID, "0197c500-0000-7000-8000-000000000012")
	body["labelIds"] = []string{}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("post status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw post response labelIds not empty; body: %s", env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	if len(res.Item.LabelIds) != 0 {
		t.Fatalf("post response labelIds = %v, want [] (explicit empty is not inherit)", res.Item.LabelIds)
	}
	if got := persistedTxLabels(t, h, tok, res.Item.ID); len(got) != 0 {
		t.Fatalf("persisted labelIds = %v, want none", got)
	}
	if got := templateLabels(t, h, tok, rtID); len(got) != 2 {
		t.Fatalf("template labelIds = %v, want both still attached", got)
	}
}

// TestPostRecurring_ExplicitForeignLabel_Rejected: the explicit list goes
// through the transaction create path's own ownership check rather than a
// second copy of that logic, so another user's label is refused here exactly
// as it is on create-transaction.
func TestPostRecurring_ExplicitForeignLabel_Rejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	rtID := postLabelsFixture(t, h, tok, "0197c400-0000-7000-8000-00000000000e")

	body := postBody(rtID, "0197c500-0000-7000-8000-000000000013")
	body["labelIds"] = []string{otherLabelID}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")

	// the refusal rolls the whole post back: the schedule must not advance
	if got := templateLabels(t, h, tok, rtID); len(got) != 2 {
		t.Fatalf("template labelIds = %v, want untouched after a rejected post", got)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	for _, it := range mustUnmarshal[recurringList](t, listEnv.Data).Items {
		if it.ID == rtID && it.NextPaymentAt != "2026-08-31 00:00:00" {
			t.Fatalf("nextPaymentAt = %q after a rejected post, want unchanged", it.NextPaymentAt)
		}
	}
}

// TestPostRecurring_TransferOverride_InheritedLabelsIgnored: post lets the
// caller override the template's type, so a labeled expense template can be
// posted as a transfer. The inherited labels ride the create request and the
// create path drops them for a transfer — "a transfer always carries an empty
// list" holds even on the inherit path, in the response and in
// transactions_labels.
func TestPostRecurring_TransferOverride_InheritedLabelsIgnored(t *testing.T) {
	h := newHarness(t)
	h.seedSecondAccount(t)
	tok := h.token(t)
	rtID := postLabelsFixture(t, h, tok, "0197c400-0000-7000-8000-00000000000f")

	body := map[string]any{
		"recurringId": rtID, "id": "0197c500-0000-7000-8000-000000000014", "type": "transfer",
		"amount": "42.50", "accountId": accountID, "accountRecipientId": recurringAcct2ID,
		"date": "2026-08-31 00:00:00",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("post status=%d body=%s", status, env.raw)
	}
	if !strings.Contains(string(env.raw), `"labelIds":[]`) {
		t.Fatalf("raw post response labelIds not empty for a transfer; body: %s", env.raw)
	}
	res := mustUnmarshal[struct {
		Item transactionItemWithLabels `json:"item"`
	}](t, env.Data)
	if len(res.Item.LabelIds) != 0 {
		t.Fatalf("transfer post labelIds = %v, want [] (inherited labels must not attach)", res.Item.LabelIds)
	}
	if got := persistedTxLabels(t, h, tok, res.Item.ID); len(got) != 0 {
		t.Fatalf("persisted labelIds = %v, want none on a transfer", got)
	}
}

// TestPostRecurring_CrossOwnerAccount_InheritedLabelsRejected: post also lets
// the caller override the template's account, so inherit is not allowed to be
// a validation bypass — the template's labels go through the create path's
// owner check against the POSTED account's owner, and a cross-owner post is
// refused exactly as an explicit list would be, instead of silently attaching
// one owner's labels to another owner's books.
func TestPostRecurring_CrossOwnerAccount_InheritedLabelsRejected(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 1, true) // role user: write access on the other owner's account
	tok := h.token(t)
	rtID := postLabelsFixture(t, h, tok, "0197c400-0000-7000-8000-000000000010")

	body := postBody(rtID, "0197c500-0000-7000-8000-000000000015")
	body["accountId"] = recSharedAcctID
	if _, ok := body["labelIds"]; ok {
		t.Fatal("this case is about labelIds being ABSENT from the body")
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")

	// the refusal rolls the whole post back: the schedule must not advance
	if got := templateLabels(t, h, tok, rtID); len(got) != 2 {
		t.Fatalf("template labelIds = %v, want untouched after a rejected post", got)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	for _, it := range mustUnmarshal[recurringList](t, listEnv.Data).Items {
		if it.ID == rtID && it.NextPaymentAt != "2026-08-31 00:00:00" {
			t.Fatalf("nextPaymentAt = %q after a rejected post, want unchanged", it.NextPaymentAt)
		}
	}
}

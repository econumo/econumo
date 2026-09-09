package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) InsertImportSource(ctx context.Context, db backend.DBTX, p insertSourceParams) error {
	return pgsqlgen.New(db).InsertImportSource(ctx, pgsqlgen.InsertImportSourceParams(p))
}

func (pgsqlQuerier) GetImportSourceByID(ctx context.Context, db backend.DBTX, id string) (sourceRow, error) {
	s, err := pgsqlgen.New(db).GetImportSourceByID(ctx, id)
	return sourceRow(s), err
}

func (pgsqlQuerier) InsertImportEvent(ctx context.Context, db backend.DBTX, p insertEventParams) (int64, error) {
	return pgsqlgen.New(db).InsertImportEvent(ctx, pgsqlgen.InsertImportEventParams(p))
}

func (pgsqlQuerier) GetImportEventByID(ctx context.Context, db backend.DBTX, id string) (eventRow, error) {
	e, err := pgsqlgen.New(db).GetImportEventByID(ctx, id)
	return eventRow(e), err
}

func (pgsqlQuerier) UpdateImportEventStatus(ctx context.Context, db backend.DBTX, p updateEventParams) error {
	return pgsqlgen.New(db).UpdateImportEventStatus(ctx, pgsqlgen.UpdateImportEventStatusParams(p))
}

func (pgsqlQuerier) InsertImportRun(ctx context.Context, db backend.DBTX, p insertRunParams) error {
	return pgsqlgen.New(db).InsertImportRun(ctx, pgsqlgen.InsertImportRunParams(p))
}

func (pgsqlQuerier) GetImportRunByID(ctx context.Context, db backend.DBTX, id string) (runRow, error) {
	r, err := pgsqlgen.New(db).GetImportRunByID(ctx, id)
	return runRow(r), err
}

func (pgsqlQuerier) UpdateImportRun(ctx context.Context, db backend.DBTX, p updateRunParams) error {
	return pgsqlgen.New(db).UpdateImportRun(ctx, pgsqlgen.UpdateImportRunParams(p))
}

func (pgsqlQuerier) InsertImportTransactionLink(ctx context.Context, db backend.DBTX, p insertLinkParams) error {
	return pgsqlgen.New(db).InsertImportTransactionLink(ctx, pgsqlgen.InsertImportTransactionLinkParams(p))
}

func (pgsqlQuerier) GetImportTransactionLinkByExternalKey(ctx context.Context, db backend.DBTX, p linkByExternalKeyPs) (linkRow, error) {
	l, err := pgsqlgen.New(db).GetImportTransactionLinkByExternalKey(ctx, pgsqlgen.GetImportTransactionLinkByExternalKeyParams(p))
	return linkRow(l), err
}

func (pgsqlQuerier) ListImportTransactionLinksByTransaction(ctx context.Context, db backend.DBTX, transactionID *string) ([]linkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportTransactionLinksByTransaction(ctx, transactionID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRow, len(rows))
	for i, row := range rows {
		out[i] = linkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) GetImportSourceByUserProvider(ctx context.Context, db backend.DBTX, p sourceByUserProviderPs) (sourceRow, error) {
	s, err := pgsqlgen.New(db).GetImportSourceByUserProvider(ctx, pgsqlgen.GetImportSourceByUserProviderParams(p))
	return sourceRow(s), err
}

func (pgsqlQuerier) ListImportSourcesByUser(ctx context.Context, db backend.DBTX, userID string) ([]sourceRow, error) {
	rows, err := pgsqlgen.New(db).ListImportSourcesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sourceRow, len(rows))
	for i, row := range rows {
		out[i] = sourceRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteImportSource(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportSource(ctx, id)
}

func (pgsqlQuerier) InsertImportAccountLink(ctx context.Context, db backend.DBTX, p insertAccountLinkParams) error {
	return pgsqlgen.New(db).InsertImportAccountLink(ctx, pgsqlgen.InsertImportAccountLinkParams(p))
}

func (pgsqlQuerier) UpdateImportAccountLink(ctx context.Context, db backend.DBTX, p updateAccountLinkParams) error {
	return pgsqlgen.New(db).UpdateImportAccountLink(ctx, pgsqlgen.UpdateImportAccountLinkParams(p))
}

func (pgsqlQuerier) DeleteImportAccountLink(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportAccountLink(ctx, id)
}

func (pgsqlQuerier) GetImportAccountLinkByID(ctx context.Context, db backend.DBTX, id string) (accountLinkRow, error) {
	a, err := pgsqlgen.New(db).GetImportAccountLinkByID(ctx, id)
	return accountLinkRow(a), err
}

func (pgsqlQuerier) ListImportAccountLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]accountLinkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportAccountLinksBySource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]accountLinkRow, len(rows))
	for i, row := range rows {
		out[i] = accountLinkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteImportEvent(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportEvent(ctx, id)
}

func (pgsqlQuerier) ListImportEventsBySourceStatus(ctx context.Context, db backend.DBTX, p eventsBySourceStatusPs) ([]eventRow, error) {
	rows, err := pgsqlgen.New(db).ListImportEventsBySourceStatus(ctx, pgsqlgen.ListImportEventsBySourceStatusParams(p))
	if err != nil {
		return nil, err
	}
	out := make([]eventRow, len(rows))
	for i, row := range rows {
		out[i] = eventRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) GetImportTransactionLinkByID(ctx context.Context, db backend.DBTX, id string) (linkRow, error) {
	l, err := pgsqlgen.New(db).GetImportTransactionLinkByID(ctx, id)
	return linkRow(l), err
}

func (pgsqlQuerier) UpdateImportTransactionLink(ctx context.Context, db backend.DBTX, p updateLinkParams) error {
	return pgsqlgen.New(db).UpdateImportTransactionLink(ctx, pgsqlgen.UpdateImportTransactionLinkParams(p))
}

func (pgsqlQuerier) ListImportTransactionLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]linkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportTransactionLinksBySource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRow, len(rows))
	for i, row := range rows {
		out[i] = linkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteQueuedImportTransactionLinksByExternalAccount(ctx context.Context, db backend.DBTX, p purgeQueuedLinksParams) error {
	return pgsqlgen.New(db).DeleteQueuedImportTransactionLinksByExternalAccount(ctx, pgsqlgen.DeleteQueuedImportTransactionLinksByExternalAccountParams(p))
}

func (pgsqlQuerier) UpdateImportSource(ctx context.Context, db backend.DBTX, p updateSourceParams) error {
	return pgsqlgen.New(db).UpdateImportSource(ctx, pgsqlgen.UpdateImportSourceParams(p))
}

func (pgsqlQuerier) UpsertImportCredentialKey(ctx context.Context, db backend.DBTX, p upsertCredentialKeyParams) error {
	return pgsqlgen.New(db).UpsertImportCredentialKey(ctx, pgsqlgen.UpsertImportCredentialKeyParams(p))
}

func (pgsqlQuerier) GetImportCredentialKey(ctx context.Context, db backend.DBTX, userID string) (credentialKeyRow, error) {
	k, err := pgsqlgen.New(db).GetImportCredentialKey(ctx, userID)
	return credentialKeyRow(k), err
}

func (pgsqlQuerier) DeleteImportCredentialKey(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteImportCredentialKey(ctx, userID)
}

func (pgsqlQuerier) ListImportRunsByUser(ctx context.Context, db backend.DBTX, p runsByUserParams) ([]runRow, error) {
	// Limit is int64 on sqlite (SQLite has no fixed-width integer bind types)
	// but pgsql's LIMIT parameter binds int32, so this is a field-by-field
	// conversion rather than the usual whole-struct cast.
	rows, err := pgsqlgen.New(db).ListImportRunsByUser(ctx, pgsqlgen.ListImportRunsByUserParams{UserID: p.UserID, Limit: int32(p.Limit)})
	if err != nil {
		return nil, err
	}
	out := make([]runRow, len(rows))
	for i, row := range rows {
		out[i] = runRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) ListImportRunsBySource(ctx context.Context, db backend.DBTX, p runsBySourceParams) ([]runRow, error) {
	rows, err := pgsqlgen.New(db).ListImportRunsBySource(ctx, pgsqlgen.ListImportRunsBySourceParams{UserID: p.UserID, SourceID: p.SourceID, Limit: int32(p.Limit)})
	if err != nil {
		return nil, err
	}
	out := make([]runRow, len(rows))
	for i, row := range rows {
		out[i] = runRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) ListImportTransactionLinksByRun(ctx context.Context, db backend.DBTX, runID *string) ([]linkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportTransactionLinksByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRow, len(rows))
	for i, row := range rows {
		out[i] = linkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) InsertImportRule(ctx context.Context, db backend.DBTX, p insertRuleParams) error {
	return pgsqlgen.New(db).InsertImportRule(ctx, pgsqlgen.InsertImportRuleParams(p))
}

func (pgsqlQuerier) UpdateImportRule(ctx context.Context, db backend.DBTX, p updateRuleParams) error {
	return pgsqlgen.New(db).UpdateImportRule(ctx, pgsqlgen.UpdateImportRuleParams(p))
}

func (pgsqlQuerier) DeleteImportRule(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportRule(ctx, id)
}

func (pgsqlQuerier) GetImportRuleByID(ctx context.Context, db backend.DBTX, id string) (ruleRow, error) {
	r, err := pgsqlgen.New(db).GetImportRuleByID(ctx, id)
	return ruleRow(r), err
}

func (pgsqlQuerier) ListImportRulesByUser(ctx context.Context, db backend.DBTX, userID string) ([]ruleRow, error) {
	rows, err := pgsqlgen.New(db).ListImportRulesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ruleRow, len(rows))
	for i, row := range rows {
		out[i] = ruleRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteImportRuleLabels(ctx context.Context, db backend.DBTX, ruleID string) error {
	return pgsqlgen.New(db).DeleteImportRuleLabels(ctx, ruleID)
}

func (pgsqlQuerier) InsertImportRuleLabel(ctx context.Context, db backend.DBTX, p insertRuleLabelParams) error {
	return pgsqlgen.New(db).InsertImportRuleLabel(ctx, pgsqlgen.InsertImportRuleLabelParams(p))
}

func (pgsqlQuerier) ListImportRuleLabels(ctx context.Context, db backend.DBTX, ruleID string) ([]ruleLabelRow, error) {
	rows, err := pgsqlgen.New(db).ListImportRuleLabels(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	out := make([]ruleLabelRow, len(rows))
	for i, row := range rows {
		out[i] = ruleLabelRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) ListImportRuleLabelsByUser(ctx context.Context, db backend.DBTX, userID string) ([]ruleLabelRow, error) {
	rows, err := pgsqlgen.New(db).ListImportRuleLabelsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ruleLabelRow, len(rows))
	for i, row := range rows {
		out[i] = ruleLabelRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteImportLinkAppliedLabels(ctx context.Context, db backend.DBTX, linkID string) error {
	return pgsqlgen.New(db).DeleteImportLinkAppliedLabels(ctx, linkID)
}

func (pgsqlQuerier) InsertImportLinkAppliedLabel(ctx context.Context, db backend.DBTX, p insertLinkAppliedParams) error {
	return pgsqlgen.New(db).InsertImportLinkAppliedLabel(ctx, pgsqlgen.InsertImportLinkAppliedLabelParams(p))
}

func (pgsqlQuerier) ListImportLinkAppliedLabels(ctx context.Context, db backend.DBTX, linkID string) ([]linkAppliedLabelRow, error) {
	rows, err := pgsqlgen.New(db).ListImportLinkAppliedLabels(ctx, linkID)
	if err != nil {
		return nil, err
	}
	out := make([]linkAppliedLabelRow, len(rows))
	for i, row := range rows {
		out[i] = linkAppliedLabelRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) ListImportTransactionLinksByUser(ctx context.Context, db backend.DBTX, userID string) ([]linkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportTransactionLinksByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRow, len(rows))
	for i, row := range rows {
		out[i] = linkRow(row)
	}
	return out, nil
}

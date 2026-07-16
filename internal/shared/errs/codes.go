package errs

// Error codes are catalogue keys under errors.* in locales/*.json. The SPA
// renders t("errors."+code, params); the English message string stays on the
// wire unchanged. Every code the backend can emit is registered in AllCodes —
// the i18ntest guard asserts a two-way match with the catalogues.
const (
	CodeIsBlank            = "common.is_blank"
	CodeInvalidChoice      = "common.invalid_choice"
	CodeInvalidFormat      = "common.invalid_format"
	CodeInvalidUUID        = "common.invalid_uuid"
	CodeInvalidEmail       = "common.invalid_email"
	CodeTooLong            = "common.too_long"
	CodeTooShort           = "common.too_short"
	CodeTooManyAttempts    = "common.too_many_attempts"
	CodeInvalidCredentials = "auth.invalid_credentials"

	// Shared across features (identical English text at more than one call
	// site) — see CLAUDE.md's "distinct English message -> distinct code"
	// rule and its "don't mint per-feature duplicates for IDENTICAL English
	// text" carve-out.
	CodeOperationLocked       = "common.operation_locked"
	CodeInvalidDatetimeFormat = "common.invalid_datetime_format"
	CodeInvalidDatetime       = "common.invalid_datetime"
	CodeIconRequired          = "common.icon_required"
	CodeFolderNameLength      = "common.folder_name_length"
	CodeInvalidCurrencyCode   = "common.invalid_currency_code"
	CodeInvalidID             = "common.invalid_id"

	CodeAccountNameLength          = "account.name_length"
	CodeAccountListEmpty           = "account.list_empty"
	CodeAccountFolderListEmpty     = "account.folder_list_empty"
	CodeAccountFolderAlreadyExists = "account.folder_already_exists"

	CodeBudgetNameLength                = "budget.name_length"
	CodeBudgetEnvelopeNameLength        = "budget.envelope_name_length"
	CodeBudgetInvalidElementTypeAlias   = "budget.invalid_element_type_alias"
	CodeBudgetInvalidRoleAlias          = "budget.invalid_role_alias"
	CodeBudgetTransactionFilterRequired = "budget.transaction_filter_required"

	CodeCategoryNameLength        = "category.name_length"
	CodeCategoryTypeInvalid       = "category.type_invalid"
	CodeCategoryReplaceIDRequired = "category.replace_id_required"
	CodeCategoryNotFound          = "category.not_found"
	CodeCategoryCannotBeReplaced  = "category.cannot_be_replaced"
	CodeCategoryListEmpty         = "category.list_empty"

	CodeConnectionInvalidUUID      = "connection.invalid_uuid"
	CodeConnectionInvalidRoleAlias = "connection.invalid_role_alias"
	CodeConnectionInvalidCode      = "connection.invalid_code"
	CodeConnectionInvitingYourself = "connection.inviting_yourself"
	CodeConnectionDeletingYourself = "connection.deleting_yourself"

	CodePayeeNameLength    = "payee.name_length"
	CodePayeeAlreadyExists = "payee.already_exists"
	CodePayeeListEmpty     = "payee.list_empty"

	CodeTagNameLength    = "tag.name_length"
	CodeTagAlreadyExists = "tag.already_exists"
	CodeTagListEmpty     = "tag.list_empty"

	CodeTokenNameLength            = "token.name_length"
	CodeTokenInvalidExpirationDate = "token.invalid_expiration_date"
	CodeTokenExpirationInFuture    = "token.expiration_in_future"
	CodeTokenRetentionNegative     = "token.retention_negative"

	CodeTransactionAccountNotAvailable = "transaction.account_not_available"
	CodeTransactionItemNotAvailable    = "transaction.item_not_available"
	CodeTransactionInvalidImportFile   = "transaction.invalid_import_file"

	CodeUserReportPeriodInvalid  = "user.report_period_invalid"
	CodeUserAlreadyExists        = "user.already_exists"
	CodeUserRegistrationDisabled = "user.registration_disabled"
	CodeUserPasswordIncorrect    = "user.password_incorrect"
	CodeUserResetPasswordError   = "user.reset_password_error"
	CodeUserResetCodeExpired     = "user.reset_code_expired"
)

var AllCodes = []string{
	CodeIsBlank,
	CodeInvalidChoice,
	CodeInvalidFormat,
	CodeInvalidUUID,
	CodeInvalidEmail,
	CodeTooLong,
	CodeTooShort,
	CodeTooManyAttempts,
	CodeInvalidCredentials,

	CodeOperationLocked,
	CodeInvalidDatetimeFormat,
	CodeInvalidDatetime,
	CodeIconRequired,
	CodeFolderNameLength,
	CodeInvalidCurrencyCode,
	CodeInvalidID,

	CodeAccountNameLength,
	CodeAccountListEmpty,
	CodeAccountFolderListEmpty,
	CodeAccountFolderAlreadyExists,

	CodeBudgetNameLength,
	CodeBudgetEnvelopeNameLength,
	CodeBudgetInvalidElementTypeAlias,
	CodeBudgetInvalidRoleAlias,
	CodeBudgetTransactionFilterRequired,

	CodeCategoryNameLength,
	CodeCategoryTypeInvalid,
	CodeCategoryReplaceIDRequired,
	CodeCategoryNotFound,
	CodeCategoryCannotBeReplaced,
	CodeCategoryListEmpty,

	CodeConnectionInvalidUUID,
	CodeConnectionInvalidRoleAlias,
	CodeConnectionInvalidCode,
	CodeConnectionInvitingYourself,
	CodeConnectionDeletingYourself,

	CodePayeeNameLength,
	CodePayeeAlreadyExists,
	CodePayeeListEmpty,

	CodeTagNameLength,
	CodeTagAlreadyExists,
	CodeTagListEmpty,

	CodeTokenNameLength,
	CodeTokenInvalidExpirationDate,
	CodeTokenExpirationInFuture,
	CodeTokenRetentionNegative,

	CodeTransactionAccountNotAvailable,
	CodeTransactionItemNotAvailable,
	CodeTransactionInvalidImportFile,

	CodeUserReportPeriodInvalid,
	CodeUserAlreadyExists,
	CodeUserRegistrationDisabled,
	CodeUserPasswordIncorrect,
	CodeUserResetPasswordError,
	CodeUserResetCodeExpired,
}

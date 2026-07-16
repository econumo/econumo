package errs

// Error codes are catalogue keys under errors.* in locales/*.json. The SPA
// renders t("errors."+code, params); the English message string stays on the
// wire unchanged. Every code the backend can emit is registered in AllCodes —
// the i18ntest guard asserts a two-way match with the catalogues.
const (
	CodeIsBlank            = "common.is_blank"
	CodeInvalidChoice      = "common.invalid_choice"
	CodeInvalidFormat      = "common.invalid_format"
	CodeValidation         = "common.validation"
	CodeInvalidUUID        = "common.invalid_uuid"
	CodeInvalidEmail       = "common.invalid_email"
	CodeTooLong            = "common.too_long"
	CodeTooShort           = "common.too_short"
	CodeTooManyAttempts    = "common.too_many_attempts"
	CodeInvalidCredentials = "auth.invalid_credentials"
)

var AllCodes = []string{
	CodeIsBlank,
	CodeInvalidChoice,
	CodeInvalidFormat,
	CodeValidation,
	CodeInvalidUUID,
	CodeInvalidEmail,
	CodeTooLong,
	CodeTooShort,
	CodeTooManyAttempts,
	CodeInvalidCredentials,
}

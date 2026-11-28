// The shape of the texts of the server's error codes (see apiErrors/): a text, or a function of what the error is
// about. The parameters arrive already written for people (a size as "10 MB", a wait as "30 秒").

export interface ErrorParams {
  // The limit that was hit, and what is used or the size that was sent.
  limit?: string
  used?: string
  size?: string
  // What it is about: a file name, a provider, a permission an App lacks, a request header, a field.
  name?: string
  provider?: string
  // The host a refused connection is about (409 partial_unsupported).
  host?: string
  permission?: string
  header?: string
  field?: string
  // The quota that ran out, and how many spaces or accounts a refusal counts.
  quota?: string
  // The feature that is off (403 feature_off), in words.
  feature?: string
  spaces?: string
  accounts?: string
  // How long to wait before asking again (Retry-After), as a phrase.
  wait?: string
}

export type ErrorText = string | ((p: ErrorParams) => string)
export type ApiErrorTexts = Record<string, ErrorText>

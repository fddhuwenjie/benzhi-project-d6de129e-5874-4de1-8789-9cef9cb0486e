package httpapi

import "net/http"

func isSuccess(code int) bool { return code >= http.StatusOK && code < http.StatusMultipleChoices }

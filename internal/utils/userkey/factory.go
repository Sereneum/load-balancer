package userkey

import (
	"errors"
	"net/http"
)

var ErrUserNotIdentified = errors.New("user not identified")

type ParamExtractorFunc func(r *http.Request) Param

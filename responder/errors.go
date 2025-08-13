package responder

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error        any    `json:"error"`
	ErrorMessage string `json:"error_message"`
	ErrorCode    int    `json:"error_code"`
}

func SendError(w http.ResponseWriter, status int, errMessage string, err ...error) {
	errResp := ErrorResponse{
		Error:        nil,
		ErrorMessage: errMessage,
		ErrorCode:    1000,
	}
	if len(err) > 0 && err[0] != nil {
		errResp.Error = err[0].Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errResp)
}

func SendErrorWithParams(w http.ResponseWriter, err string, status int, errorCode *int, errorMessage *string) {
	errResp := ErrorResponse{
		Error:        err,
		ErrorMessage: "",
		ErrorCode:    1000,
	}

	if errorCode != nil && *errorCode > 0 {
		errResp.ErrorCode = *errorCode
	}

	if errorMessage != nil && len(*errorMessage) > 0 {
		errResp.ErrorMessage = *errorMessage
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errResp)
}

func BadBody(w http.ResponseWriter, err error) {
	SendError(w, http.StatusBadRequest, "bad body", err)
}

func ErrMissingBodyRequirement(w http.ResponseWriter, field string) {
	SendError(w, http.StatusBadRequest, "missing required body field: "+field)
}

func ErrInvalidBodyField(w http.ResponseWriter, field string, err error) {
	SendError(w, http.StatusBadRequest, "invalid body field: "+field, err)
}

func ErrConflict(w http.ResponseWriter, err error) {
	SendError(w, http.StatusConflict, "conflict", err)
}

func ErrInternal(w http.ResponseWriter, err error) {
	SendError(w, http.StatusInternalServerError, "internal server error", err)
}

func ParamError(w http.ResponseWriter, field string) {
	SendError(w, http.StatusBadRequest, "missing required param: "+field)
}

func ErrRequiredKey(w http.ResponseWriter, key string) {
	SendError(w, http.StatusBadRequest, "missing required key: "+key)
}

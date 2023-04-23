package errors

import (
	"encoding/json"
	"net/http"

	"github.com/hillview.tv/videoAPI/responder"
)

type ErrorResponse struct {
	Error        string `json:"error"`
	ErrorMessage string `json:"error_message"`
	ErrorCode    int    `json:"error_code"`
}

func SendError(w http.ResponseWriter, err string, status int) {
	errResp := ErrorResponse{
		Error:        err,
		ErrorMessage: "",
		ErrorCode:    1000,
	}

	w.WriteHeader(status)
	json.NewEncoder(w).Encode(responder.Error(errResp))
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

	w.WriteHeader(status)
	json.NewEncoder(w).Encode(responder.Error(errResp))
}

func ParamError(w http.ResponseWriter, field string) {
	SendError(w, "missing required param: "+field, http.StatusBadRequest)
}

func ErrRequiredKey(w http.ResponseWriter, key string) {
	SendError(w, "missing required key: "+key, http.StatusBadRequest)
}

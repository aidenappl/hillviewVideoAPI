package routers

import (
	"encoding/json"
	"net/http"
	"net/mail"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/errors"
	"github.com/hillview.tv/videoAPI/query"
)

type HandlePostNewsletterRequest struct {
	Email *string `json:"email"`
}

func HandlePostNewsletter(w http.ResponseWriter, r *http.Request) {
	// Parse the body of the request
	body := HandlePostNewsletterRequest{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		errors.SendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate that the email exists
	if body.Email == nil || len(*body.Email) == 0 {
		errors.SendError(w, "missing email in the body", http.StatusBadRequest)
		return
	}

	// Validate email
	_, err = mail.ParseAddress(*body.Email)
	if err != nil {
		errors.SendError(w, "invalid email address in the body", http.StatusBadRequest)
		return
	}

	// Create a new newsletter entry
	err = query.CreateNewsletterSignup(db.DB, query.CreateNewsletterSignupRequest{
		Email: body.Email,
	})
	if err != nil {
		errors.SendError(w, err.Error(), http.StatusConflict)
		return
	}

	// Success response
	w.WriteHeader(http.StatusNoContent)

}

package routers

import (
	"encoding/json"
	"net/http"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/errors"
	"github.com/hillview.tv/videoAPI/mailer"
	"github.com/hillview.tv/videoAPI/query"
)

type HandleUnsubscribeNewsletterRequest struct {
	Email *string `json:"email"`
}

func HandleUnsubscribeNewsletter(w http.ResponseWriter, r *http.Request) {
	// build the body
	body := HandleUnsubscribeNewsletterRequest{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		errors.SendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// validate the email exists
	if body.Email == nil || len(*body.Email) == 0 {
		errors.SendError(w, "missing email in the body", http.StatusBadRequest)
		return
	}

	// unsubscribe the email from database
	err = query.UnsubscribeNewsletter(db.DB, query.UnsubscribeNewsletterRequest{
		Email: body.Email,
	})
	if err != nil {
		errors.SendError(w, err.Error(), http.StatusConflict)
		return
	}

	// unsubscribe the email from sendgrid
	err = mailer.GlobalUnsubscribe(*body.Email)
	if err != nil {
		errors.SendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)

}

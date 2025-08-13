package routers

import (
	"encoding/json"
	"net/http"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/mailer"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
)

type HandleUnsubscribeNewsletterRequest struct {
	Email *string `json:"email"`
}

func HandleUnsubscribeNewsletter(w http.ResponseWriter, r *http.Request) {
	// build the body
	body := HandleUnsubscribeNewsletterRequest{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		responder.BadBody(w, err)
		return
	}

	// validate the email exists
	if body.Email == nil || len(*body.Email) == 0 {
		responder.ErrMissingBodyRequirement(w, "email")
		return
	}

	// unsubscribe the email from database
	err = query.UnsubscribeNewsletter(db.DB, query.UnsubscribeNewsletterRequest{
		Email: body.Email,
	})
	if err != nil {
		responder.ErrConflict(w, err)
		return
	}

	// unsubscribe the email from sendgrid
	err = mailer.GlobalUnsubscribe(*body.Email)
	if err != nil {
		responder.ErrInternal(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

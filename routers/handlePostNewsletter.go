package routers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/mail"

	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/mailer"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
)

type HandlePostNewsletterRequest struct {
	Email *string `json:"email"`
}

func HandlePostNewsletter(w http.ResponseWriter, r *http.Request) {
	// Parse the body of the request
	body := HandlePostNewsletterRequest{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		responder.BadBody(w, err)
		return
	}

	// Validate that the email exists
	if body.Email == nil || len(*body.Email) == 0 {
		responder.ErrMissingBodyRequirement(w, "email")
		return
	}

	// Validate email
	_, err = mail.ParseAddress(*body.Email)
	if err != nil {
		responder.ErrInvalidBodyField(w, "email", err)
		return
	}

	// Create a new newsletter entry
	err = query.CreateNewsletterSignup(db.DB, query.CreateNewsletterSignupRequest{
		Email: body.Email,
	})
	if err != nil {
		responder.ErrConflict(w, err)
		return
	}

	// Send confirmation email
	_, err = mailer.SendTemplate(mailer.SendTemplateRequest{
		TemplateID: "d-d9c9c4be63c74755b3512084c96e5da6",
		FromEmail:  "notifications@hillview.tv",
		FromName:   "HillviewTV Notifications",
		ToName:     *body.Email,
		ToEmail:    *body.Email,
		DynamicData: map[string]interface{}{
			"title":             "You're On The List!",
			"body":              "Hello!\n\nYou've successfully signed up for HillviewTV notifications. For now, we're only notifying you when we upload new playlists for MPCSD Drama Productions, but we'll give you more controls for additional curation in the future!\n\nYou can unsubscribe any time at the link at the bottom of this and any other HillviewTV newsletter email.\n\nThanks and enjoy the content!",
			"action_button_url": "https://hillview.tv/content",
			"email":             body.Email,
		},
	})
	if err != nil {
		log.Println("failed to send email! ", err.Error())
	}

	// Success response
	w.WriteHeader(http.StatusNoContent)
}

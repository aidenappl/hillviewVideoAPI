package mailer

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/hillview.tv/videoAPI/env"
)

func GlobalUnsubscribe(email string) error {
	// create a new request
	req, err := http.NewRequest(http.MethodPost, "https://api.sendgrid.com/v3/asm/suppressions/global", bytes.NewBuffer([]byte(`{"recipient_emails": ["`+email+`"]}`)))
	if err != nil {
		return err
	}

	// add the necessary headers
	req.Header.Add("Authorization", "Bearer "+env.SendgridAPIKey)
	req.Header.Add("Content-Type", "application/json")

	// send the request
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	// check if the email was successfully unsubscribed
	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to unsubscribe email: %s", email)
	}

	// return nil if everything went well
	return nil
}

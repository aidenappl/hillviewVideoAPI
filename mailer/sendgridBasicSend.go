package mailer

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type SendBasicMailRequest struct {
	FromEmail   string
	FromName    string
	Subject     string
	ToName      string
	ToEmail     string
	PlainText   string
	HTMLContent string
	InReplyTo   *string
	MessageUUID *string
}

func SendBasic(req SendBasicMailRequest) (*rest.Response, error) {

	client := GetSendgridClient()

	if req.FromEmail == "" || req.FromName == "" || req.ToEmail == "" || req.ToName == "" || req.Subject == "" {
		return nil, fmt.Errorf("incorrectly formatted mailer request, please review")
	}

	from := mail.NewEmail(req.FromName, req.FromEmail)
	to := mail.NewEmail(req.ToName, req.ToEmail)
	message := mail.NewSingleEmail(from, req.Subject, to, req.PlainText, req.HTMLContent)

	if req.MessageUUID == nil {
		uuid := uuid.New().String()
		req.MessageUUID = &uuid
	}

	message.Headers = make(map[string]string)
	message.Headers["Message-ID"] = "<" + *req.MessageUUID + "@trailblaze.to>"
	if req.InReplyTo != nil {
		message.Headers["In-Reply-To"] = *req.InReplyTo
	}

	p := mail.NewPersonalization()
	p.AddTos(to)
	p.Headers = message.Headers
	message.AddPersonalizations(p)
	response, err := client.Send(message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message:", err)
	}

	return response, nil
}

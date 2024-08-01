package mailer

import (
	"fmt"

	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type SendTemplateRequest struct {
	TemplateID  string
	FromEmail   string
	FromName    string
	ToName      string
	ToEmail     string
	DynamicData map[string]interface{}
}

func SendTemplate(req SendTemplateRequest) (*rest.Response, error) {

	from := mail.NewEmail(req.FromName, req.FromEmail)
	to := mail.NewEmail(req.ToName, req.ToEmail)

	personalizations := mail.NewPersonalization()
	personalizations.To = append(personalizations.To, to)
	personalizations.DynamicTemplateData = req.DynamicData

	mailer := mail.NewV3Mail()
	mailer.SetFrom(from)
	mailer.SetTemplateID(req.TemplateID)
	mailer.Personalizations = append(mailer.Personalizations, personalizations)

	client := GetSendgridClient()

	response, err := client.Send(mailer)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: ", err)
	}
	return response, nil
}

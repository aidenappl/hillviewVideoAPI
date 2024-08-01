package mailer

import (
	"github.com/hillview.tv/videoAPI/env"
	"github.com/sendgrid/sendgrid-go"
)

func GetSendgridClient() *sendgrid.Client {
	client := sendgrid.NewSendClient(env.SendgridAPIKey)
	return client
}

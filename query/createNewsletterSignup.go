package query

import (
	"fmt"
	"net/mail"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
)

type CreateNewsletterSignupRequest struct {
	Email *string `json:"email"`
}

func CreateNewsletterSignup(db db.Queryable, req CreateNewsletterSignupRequest) error {
	if req.Email == nil || len(*req.Email) == 0 {
		return fmt.Errorf("missing email in query request")
	}

	// Validate email
	_, err := mail.ParseAddress(*req.Email)
	if err != nil {
		return fmt.Errorf("invalid email address")
	}

	query, args, err := sq.Insert("newsletter").Columns("email").Values(req.Email).ToSql()
	if err != nil {
		return fmt.Errorf("failed to create query: %w", err)
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return fmt.Errorf("this email has already been registered")
		}
		return fmt.Errorf("failed to execute query: %w", err)
	}

	_, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	return nil
}

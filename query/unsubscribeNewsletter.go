package query

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
)

type UnsubscribeNewsletterRequest struct {
	Email *string `json:"email"`
}

func UnsubscribeNewsletter(db db.Queryable, req UnsubscribeNewsletterRequest) error {
	query, args, err := sq.Update("newsletter").Set("subscribed", false).Where(sq.Eq{"email": req.Email}).ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update newsletter: %w", err)
	}

	return err
}

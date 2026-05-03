package query

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
)

type RecordViewRequest struct {
	ID        int
	IPAddress *string
}

func RecordView(db db.Queryable, req RecordViewRequest) error {
	// validate required fields
	if req.ID == 0 {
		return fmt.Errorf("id is required")
	}

	cols := []string{"video_id"}
	vals := []interface{}{req.ID}

	if req.IPAddress != nil {
		cols = append(cols, "ip_address")
		vals = append(vals, req.IPAddress)
	}

	query, args, err := sq.Insert("video_views").
		Columns(cols...).
		Values(vals...).
		ToSql()
	if err != nil {
		return fmt.Errorf("error building query: %v", err)
	}

	_, err = db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("error executing query: %v", err)
	}

	return nil
}

package query

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
)

type RecordDownloadRequest struct {
	ID int `json:"id"`
}

func RecordDownload(db db.Queryable, req RecordDownloadRequest) error {
	// validate required fields
	if req.ID == 0 {
		return fmt.Errorf("id is required")
	}

	// insert new view
	query, args, err := sq.Insert("video_downloads").
		Columns("video_id").
		Values(req.ID).
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

package query

import (
	"database/sql"
	"fmt"

	"github.com/hillview.tv/videoAPI/db"
)

type Caption struct {
	Language string  `json:"language"`
	Status   string  `json:"status"`
	VTT      *string `json:"vtt"`
}

// GetCaption returns the stored caption for a video+language, or nil if none.
func GetCaption(database db.Queryable, videoID int, language string) (*Caption, error) {
	row := database.QueryRow(
		"SELECT language, status, vtt FROM video_captions WHERE video_id = ? AND language = ? LIMIT 1",
		videoID, language,
	)
	var c Caption
	err := row.Scan(&c.Language, &c.Status, &c.VTT)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan caption: %w", err)
	}
	return &c, nil
}

// UpsertCaption inserts or updates a caption row's status and content.
func UpsertCaption(database db.Queryable, videoID int, language, status string, vtt, plainText *string) error {
	_, err := database.Exec(
		`INSERT INTO video_captions (video_id, language, status, vtt, plain_text)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE status = VALUES(status), vtt = VALUES(vtt), plain_text = VALUES(plain_text)`,
		videoID, language, status, vtt, plainText,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert caption: %w", err)
	}
	return nil
}

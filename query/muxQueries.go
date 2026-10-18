package query

import (
	"database/sql"
	"fmt"

	"github.com/hillview.tv/videoAPI/db"
)

// GetVideoMuxIDs returns the Mux asset and playback ids stored on a video
// ("" when unset). The url stays the source of truth: a stored playback id
// that no longer matches the url is stale (the url was edited).
func GetVideoMuxIDs(database db.Queryable, videoID int) (assetID string, playbackID string, err error) {
	var a, p sql.NullString
	err = database.QueryRow("SELECT mux_asset_id, mux_playback_id FROM videos WHERE id = ?", videoID).Scan(&a, &p)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to read mux ids: %w", err)
	}
	return a.String, p.String, nil
}

// SetVideoMuxIDs stores the Mux asset and playback ids for a video.
func SetVideoMuxIDs(database db.Queryable, videoID int, assetID, playbackID string) error {
	if _, err := database.Exec(
		"UPDATE videos SET mux_asset_id = ?, mux_playback_id = ? WHERE id = ?",
		assetID, playbackID, videoID,
	); err != nil {
		return fmt.Errorf("failed to store mux ids: %w", err)
	}
	return nil
}

// MuxVideo is a video row that plays a given Mux asset.
type MuxVideo struct {
	ID  int
	URL string
}

// FindVideosByMuxAsset returns the videos playing a Mux asset: rows whose
// stored asset id matches, or whose url is the asset's playback url (a row
// created before its ids were resolved).
func FindVideosByMuxAsset(database db.Queryable, assetID, playbackURL string) ([]MuxVideo, error) {
	rows, err := database.Query(
		"SELECT id, url FROM videos WHERE mux_asset_id = ? OR url = ?",
		assetID, playbackURL,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find videos by mux asset: %w", err)
	}
	defer rows.Close()

	var videos []MuxVideo
	for rows.Next() {
		var v MuxVideo
		if err := rows.Scan(&v.ID, &v.URL); err != nil {
			return nil, fmt.Errorf("failed to scan video: %w", err)
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

// GetCaptionVTTByVideoUUID returns a video's ready caption VTT, or nil. It
// ignores video status: Mux fetches edited captions from it while a video is
// still a draft.
func GetCaptionVTTByVideoUUID(database db.Queryable, uuid, language string) (*string, error) {
	var vtt sql.NullString
	err := database.QueryRow(
		`SELECT vc.vtt FROM video_captions vc
		   JOIN videos v ON v.id = vc.video_id
		  WHERE v.uuid = ? AND vc.language = ? AND vc.status = 'ready'
		  LIMIT 1`,
		uuid, language,
	).Scan(&vtt)
	if err == sql.ErrNoRows || (err == nil && !vtt.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read caption vtt: %w", err)
	}
	return &vtt.String, nil
}

package query

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/structs"
)

type ListSpotlightsRequest struct {
	Limit  *int
	Offset *int
}

func ListSpotlights(db db.Queryable, req ListSpotlightsRequest) ([]*structs.Spotlight, error) {
	//  check required fields
	if req.Limit == nil {
		return nil, fmt.Errorf("required field limit is nil")
	}

	if req.Offset == nil {
		return nil, fmt.Errorf("required field offset is nil")
	}

	// build query

	q := sq.Select(
		"spotlight.rank",
		"spotlight.video_id",
		"spotlight.inserted_at",
		"spotlight.updated_at",

		"videos.id",
		"videos.uuid",
		"videos.title",
		"videos.description",
		"videos.thumbnail",
		"videos.url",
		"videos.download_url",
		"videos.allow_downloads",
		"videos.inserted_at",
		`(
			SELECT COUNT(video_views.id) FROM video_views WHERE video_views.video_id = videos.id
		) as views`,

		"video_statuses.id",
		"video_statuses.name",
		"video_statuses.short_name",
	)

	q = q.From("spotlight").
		LeftJoin("videos ON spotlight.video_id = videos.id").
		LeftJoin("video_statuses ON videos.status = video_statuses.id").
		Where("videos.status = 1").
		OrderBy("spotlight.rank ASC").
		Limit(uint64(*req.Limit)).
		Offset(uint64(*req.Offset))

	query, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %v", err)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}

	defer rows.Close()

	spotlights := []*structs.Spotlight{}
	for rows.Next() {
		s := &structs.Spotlight{}
		v := &structs.NulledVideo{}
		v.Status = &structs.GeneralNSNNulled{}
		vid := 0

		err := rows.Scan(
			&s.Rank,
			&vid,
			&s.InsertedAt,
			&s.UpdatedAt,

			&v.ID,
			&v.UUID,
			&v.Title,
			&v.Description,
			&v.Thumbnail,
			&v.URL,
			&v.DownloadURL,
			&v.AllowDownloads,
			&v.InsertedAt,
			&v.Views,

			&v.Status.ID,
			&v.Status.Name,
			&v.Status.ShortName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}

		if v.ID != nil {
			s.Video = &structs.Video{
				UUID:           *v.UUID,
				Title:          *v.Title,
				Description:    *v.Description,
				Thumbnail:      *v.Thumbnail,
				URL:            *v.URL,
				DownloadURL:    v.DownloadURL,
				AllowDownloads: *v.AllowDownloads,
				InsertedAt:     *v.InsertedAt,
				Status: &structs.GeneralNSN{
					ID:        *v.Status.ID,
					Name:      *v.Status.Name,
					ShortName: *v.Status.ShortName,
				},
			}
			spotlights = append(spotlights, s)
		}
	}

	return spotlights, nil

}

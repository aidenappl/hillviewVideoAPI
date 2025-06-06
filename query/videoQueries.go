package query

import (
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/structs"
)

type GetVideoRequest struct {
	ID   *int    `json:"id"`
	UUID *string `json:"uuid"`
}

func GetVideo(db db.Queryable, req GetVideoRequest) (*structs.Video, error) {
	// check the req
	if req.ID == nil && req.UUID == nil {
		return nil, fmt.Errorf("missing 'id' or 'uuid' query param")
	}

	// create the query
	q := sq.Select(
		"videos.id",
		"videos.uuid",
		"videos.title",
		"videos.description",
		"videos.thumbnail",
		"videos.url",
		"videos.download_url",
		"videos.allow_downloads",
		"videos.inserted_at",

		"video_statuses.id",
		"video_statuses.name",
		"video_statuses.short_name",
	).
		From("videos").
		LeftJoin("video_statuses ON videos.status = video_statuses.id").
		Where(sq.Or{sq.Eq{"videos.status": 1}, sq.Eq{"videos.status": 3}}).
		OrderBy("videos.id DESC")

	if req.ID != nil {
		q = q.Where(sq.Eq{"videos.id": *req.ID})
	}

	if req.UUID != nil {
		q = q.Where(sq.Eq{"videos.uuid": *req.UUID})
	}

	query, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to create query: %w", err)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	var video structs.Video
	var status structs.GeneralNSN

	err = rows.Scan(
		&video.ID,
		&video.UUID,
		&video.Title,
		&video.Description,
		&video.Thumbnail,
		&video.URL,
		&video.DownloadURL,
		&video.AllowDownloads,
		&video.InsertedAt,

		&status.ID,
		&status.Name,
		&status.ShortName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	if strings.Contains(video.URL, "cloudflare") {
		parts := strings.Split(video.URL, "/")
		if len(parts) > 0 {
			video.CloudflareID = &parts[len(parts)-3]
		} else {
			video.CloudflareID = nil
		}
	}

	video.Status = &status

	return &video, nil
}

type ListVideosRequest struct {
	Limit      *uint64
	Offset     *uint64
	Search     *string
	Sort       *string
	By         *string
	PlaylistID *int
}

func ListVideos(db db.Queryable, req ListVideosRequest) ([]*structs.Video, error) {

	if req.Limit == nil {
		req.Limit = new(uint64)
		*req.Limit = 10
	}

	if req.Offset == nil {
		req.Offset = new(uint64)
		*req.Offset = 0
	}

	if req.Sort != nil {
		*req.Sort = strings.ToLower(*req.Sort)
		if *req.Sort != "asc" && *req.Sort != "desc" {
			return nil, fmt.Errorf("invalid sort value: %s", *req.Sort)
		}
	} else {
		req.Sort = new(string)
		*req.Sort = "desc"
	}

	if req.By != nil {
		*req.By = strings.ToLower(*req.By)
		if *req.By != "date" && *req.By != "views" {
			return nil, fmt.Errorf("invalid by value: %s", *req.By)
		}
	} else {
		req.By = new(string)
		*req.By = "date"
	}

	q := sq.Select(
		"videos.id",
		"videos.uuid",
		"videos.title",
		"videos.description",
		"videos.thumbnail",
		"videos.url",
		"videos.download_url",
		"videos.allow_downloads",
		"videos.inserted_at",

		"video_statuses.id",
		"video_statuses.name",
		"video_statuses.short_name",

		"COUNT(video_views.video_id) as views",
	).
		From("videos").
		LeftJoin("video_statuses ON videos.status = video_statuses.id").
		LeftJoin("video_views ON videos.id = video_views.video_id").
		GroupBy("videos.id").
		Where(sq.Eq{"video_statuses.id": 1}).
		Limit(*req.Limit).
		Offset(*req.Offset)

	if *req.By == "date" {
		q = q.OrderBy("videos.inserted_at " + *req.Sort)
	} else if *req.By == "views" {
		q = q.OrderBy("views " + *req.Sort)
	} else {
		q = q.OrderBy("videos.id " + *req.Sort)
	}

	if req.Search != nil {
		q = q.Where(
			sq.Or{
				sq.Like{"videos.title": "%" + string(*req.Search) + "%"},
				sq.Like{"videos.description": "%" + string(*req.Search) + "%"},
			},
		)
	}

	if req.PlaylistID != nil {
		q = q.Join("playlist_associations pa on videos.id = pa.video_id")
		q = q.Where(sq.Eq{"pa.playlist_id": *req.PlaylistID})
	}

	query, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to create query: %w", err)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	defer rows.Close()

	var videos []*structs.Video

	for rows.Next() {
		var video structs.Video
		var status structs.GeneralNSN

		err = rows.Scan(
			&video.ID,
			&video.UUID,
			&video.Title,
			&video.Description,
			&video.Thumbnail,
			&video.URL,
			&video.DownloadURL,
			&video.AllowDownloads,
			&video.InsertedAt,

			&status.ID,
			&status.Name,
			&status.ShortName,

			&video.Views,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if strings.Contains(video.URL, "cloudflare") {
			parts := strings.Split(video.URL, "/")
			if len(parts) > 0 {
				video.CloudflareID = &parts[len(parts)-3]
			} else {
				video.CloudflareID = nil
			}
		}

		video.Status = &status

		videos = append(videos, &video)

	}

	return videos, nil
}

type CreateVideoRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Thumbnail   *string `json:"thumbnail"`
	URL         *string `json:"url"`
}

func CreateVideo(db db.Queryable, req CreateVideoRequest) (*structs.Video, error) {

	query, args, err := sq.Insert("videos").
		Columns(
			"title",
			"description",
			"thumbnail",
			"url",
		).
		Values(
			req.Title,
			req.Description,
			req.Thumbnail,
			req.URL,
		).
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("failed to create query: %w", err)
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert id: %w", err)
	}

	j := int(id)

	return GetVideo(db, GetVideoRequest{ID: &j})

}

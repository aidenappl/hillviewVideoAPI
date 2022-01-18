package query

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/structs"
)

type GetVideoRequest struct {
	ID *int `json:"id"`
}

func GetVideo(db db.Queryable, req GetVideoRequest) (*structs.Video, error) {
	query, args, err := sq.Select(
		"videos.id",
		"videos.title",
		"videos.description",
		"videos.thumbnail",
		"videos.url",
		"videos.inserted_at",

		"video_statuses.id",
		"video_statuses.name",
		"video_statuses.short_name",
	).
		From("videos").
		LeftJoin("video_statuses ON videos.status = video_statuses.id").
		OrderBy("videos.id DESC").
		Where(sq.Eq{"videos.id": req.ID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to create query: %w", err)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("no video found")
	}

	var video structs.Video
	var status structs.GeneralNSN

	err = rows.Scan(
		&video.ID,
		&video.Title,
		&video.Description,
		&video.Thumbnail,
		&video.URL,
		&video.InsertedAt,

		&status.ID,
		&status.Name,
		&status.ShortName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	video.Status = &status

	return &video, nil
}

type ListVideosRequest struct {
	Limit *uint64
}

func ListVideos(db db.Queryable, req ListVideosRequest) ([]*structs.Video, error) {

	query, args, err := sq.Select(
		"videos.id",
		"videos.title",
		"videos.description",
		"videos.thumbnail",
		"videos.url",
		"videos.inserted_at",

		"video_statuses.id",
		"video_statuses.name",
		"video_statuses.short_name",
	).
		From("videos").
		LeftJoin("video_statuses ON videos.status = video_statuses.id").
		OrderBy("videos.id ASC").
		Limit(*req.Limit).
		ToSql()
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
			&video.Title,
			&video.Description,
			&video.Thumbnail,
			&video.URL,
			&video.InsertedAt,

			&status.ID,
			&status.Name,
			&status.ShortName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
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

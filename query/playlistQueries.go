package query

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/structs"
)

type ListPlaylistsRequest struct {
	Limit  uint64
	Offset uint64
}

func ListPlaylists(db db.Queryable, req ListPlaylistsRequest) ([]structs.Playlist, error) {

	var playlists []structs.Playlist

	q := sq.Select(
		"playlists.id",
		"playlists.name",
		"playlists.description",
		"playlists.banner_image",
		"playlists.route",
		"playlists.inserted_at",
	).
		From("playlists")

	query, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("error parsing query: %s", err)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying playlists: %s", err)
	}

	for rows.Next() {
		var playlist structs.Playlist
		err := rows.Scan(
			&playlist.ID,
			&playlist.Name,
			&playlist.Description,
			&playlist.BannerImage,
			&playlist.Route,
			&playlist.InsertedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning playlist: %s", err)
		}

		videos, err := ListVideos(db, ListVideosRequest{
			PlaylistID: &playlist.ID,
			Limit:      &req.Limit,
			Offset:     &req.Offset,
		})
		if err != nil {
			return nil, fmt.Errorf("error querying videos: %s", err)
		}

		playlist.Videos = videos

		playlists = append(playlists, playlist)
	}

	return playlists, nil
}

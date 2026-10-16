package routers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/awsBridge"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/downloads"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
	"github.com/hillview.tv/videoAPI/telemetry"
)

// DOWNLOAD_LINK_TTL is how long a presigned S3 download link stays valid.
const DOWNLOAD_LINK_TTL = 15 * time.Minute

// HandleDownloadVideo answers GET /download/{query} with a 302 to a URL the
// browser saves as "<video title>.mp4". The bytes come straight from S3 or
// Cloudflare Stream — never through this API or the web server, which sit on
// the studio's single WAN.
func HandleDownloadVideo(w http.ResponseWriter, r *http.Request) {
	q := mux.Vars(r)["query"]
	req := query.GetVideoRequest{}
	if id, err := strconv.Atoi(q); err == nil {
		req.ID = &id
	} else {
		req.UUID = &q
	}

	video, err := query.GetVideo(db.DB, req)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to execute query", err)
		return
	}
	if video == nil {
		responder.SendError(w, http.StatusNotFound, "video not found")
		return
	}
	if !video.AllowDownloads || video.DownloadURL == nil || *video.DownloadURL == "" {
		responder.SendError(w, http.StatusForbidden, "downloads are not enabled for this video")
		return
	}

	target, err := downloads.Target(*video.DownloadURL, video.Title, presignContent)
	if err != nil {
		// Still serve the file; the browser just names it after the URL.
		telemetry.Error(r.Context(), "video.download.sign_failed", err, map[string]any{"video_id": video.ID})
		target = *video.DownloadURL
	}

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

// presignContent signs a GET for the content bucket. It is signed for the
// bucket's own region (the API session's region differs) and path-style,
// because the bucket name has dots, which break TLS on virtual-hosted URLs.
func presignContent(key string, disposition string) (string, error) {
	client := s3.New(awsBridge.Connect(), aws.NewConfig().
		WithRegion(downloads.CONTENT_BUCKET_REGION).
		WithS3ForcePathStyle(true))
	req, _ := client.GetObjectRequest(&s3.GetObjectInput{
		Bucket:                     aws.String(downloads.CONTENT_BUCKET),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(disposition),
		ResponseContentType:        aws.String("video/mp4"),
	})
	return req.Presign(DOWNLOAD_LINK_TTL)
}

package routers

import (
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/awsBridge"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/middleware"
	"github.com/hillview.tv/videoAPI/mux"
	"github.com/hillview.tv/videoAPI/responder"
	"github.com/hillview.tv/videoAPI/streamurl"
)

const letterBytes = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func RandStringBytesMaskImpr(n int) string {
	b := make([]byte, n)
	// A rand.Int63() generates 63 random bits, enough for letterIdxMax letters!
	for i, cache, remain := n-1, rand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

type VideoUploadResponse struct {
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	S3Url     string `json:"s3_url"`
	Duration  int    `json:"duration"`
}

func HandleVideoUpload(w http.ResponseWriter, r *http.Request) {

	resetMultipart := func(w http.ResponseWriter) {
		files, err := os.ReadDir("/tmp")
		if err != nil {
			responder.SendError(w, http.StatusBadRequest, "failed to clear tmp", err)
			return
		}
		res := &[]string{}
		for _, f := range files {
			if !f.IsDir() && strings.HasPrefix(f.Name(), "multipart-") {
				*res = append(*res, f.Name())
				os.Remove(filepath.Join("/tmp", f.Name()))
			}
		}
	}

	// Get the claims from the context
	claims := middleware.WithClaimsValue(r.Context())
	if claims == nil {
		responder.SendError(w, http.StatusUnauthorized, "Missing Authorization token")
		resetMultipart(w)
		return
	}

	// Convert the subject to an int
	sub, err := strconv.Atoi(claims.Subject)
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to get subject", err)
		resetMultipart(w)
		return
	}

	// Get the file from the form data
	file, fileHeader, err := r.FormFile("upload")
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to get formfile", err)
		resetMultipart(w)
		return
	}
	defer file.Close()

	log.Println("Successfully got an upload from the form data")

	// Generate a unique name for the file
	generated := "UID" + strconv.Itoa(sub) + "-" + strconv.FormatInt(time.Now().Unix(), 10) + "-" + RandStringBytesMaskImpr(10) + ".mp4"
	log.Println("Generated name:", generated)

	// Upload the temporary file to s3
	response, err := actions.UploadMultipart(file, fileHeader, generated)
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to upload to s3", err)
		resetMultipart(w)
		return
	}

	file.Close()

	log.Println("Successfully uploaded a video to S3:", *response.Location)

	s3URL := "https://content.hillview.tv/videos/uploads/" + generated

	// Ingest the S3 object into Mux. The playback id exists immediately;
	// duration arrives with video.asset.ready (and the duration sync).
	asset, err := actions.Mux.CreateAsset(mux.NewAssetSettings(s3URL, "user:"+strconv.Itoa(sub), true))
	if err != nil {
		resetMultipart(w)
		sendMuxError(w, "failed to create Mux asset", err)
		return
	}
	pid := asset.PublicPlaybackID()

	resetMultipart(w)

	responder.New(w, VideoUploadResponse{
		URL:       streamurl.MuxURL(pid),
		Thumbnail: mux.ThumbnailURL(pid, nil),
		S3Url:     s3URL,
		Duration:  0,
	})
}

type ThumbnailUploadResponse struct {
	URL string `json:"url"`
}

func HandleThumbnailUpload(w http.ResponseWriter, r *http.Request) {

	claims := middleware.WithClaimsValue(r.Context())
	if claims == nil {
		responder.ParamError(w, "Authorization")
		return
	}

	sub, err := strconv.Atoi(claims.Subject)
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to get subject", err)
		return
	}

	sessionHandler := awsBridge.ContentSession()
	uploader := s3manager.NewUploader(sessionHandler)
	file, _, err := r.FormFile("upload")
	if err != nil {
		responder.BadBody(w, err)
		return
	}

	now := time.Now()
	sec := now.Unix()
	generated := "UID" + strconv.Itoa(sub) + "-" + strconv.FormatInt(sec, 10) + "-" + RandStringBytesMaskImpr(10) + ".jpg"

	//upload to the s3 bucket
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(env.ContentS3Bucket),
		ACL:         aws.String("public-read"),
		Key:         aws.String("thumbnails/" + *aws.String(generated)),
		Body:        file,
		ContentType: aws.String("image/jpeg"),
	})
	if err != nil {
		responder.ErrInternal(w, err)
		return
	}

	responder.New(w, ThumbnailUploadResponse{
		URL: env.ContentPublicBaseURL + "/thumbnails/" + generated,
	}, "Thumbnail uploaded successfully")

}

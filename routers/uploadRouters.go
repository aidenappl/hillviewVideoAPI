package routers

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hillview.tv/videoAPI/awsBridge"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/middleware"
	"github.com/hillview.tv/videoAPI/responder"
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

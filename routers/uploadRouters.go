package routers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hillview.tv/videoAPI/awsBridge"
	"github.com/hillview.tv/videoAPI/middleware"
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

type VideoUplaodResponse struct {
	URL string `json:"url"`
}

func HandleVideoUpload(w http.ResponseWriter, r *http.Request) {

	claims := middleware.WithClaimsValue(r.Context())
	if claims == nil {
		http.Error(w, "Missing Authorization token", http.StatusUnauthorized)
		return
	}

	sub, err := strconv.Atoi(claims.Subject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionHandler := awsBridge.Connect()
	uploader := s3manager.NewUploader(sessionHandler)
	file, _, err := r.FormFile("upload")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	sec := now.Unix()
	generated := "UID" + strconv.Itoa(sub) + "-" + strconv.FormatInt(sec, 10) + "-" + RandStringBytesMaskImpr(10) + ".mp4"

	//upload to the s3 bucket
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String("content.hillview.tv"),
		ACL:         aws.String("public-read"),
		Key:         aws.String("videos/uploads/" + *aws.String(generated)),
		Body:        file,
		ContentType: aws.String("video/mp4"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	//return the url of the uploaded file
	json.NewEncoder(w).Encode(VideoUplaodResponse{
		URL: "https://content.hillview.tv/videos/uploads/" + generated,
	})
}

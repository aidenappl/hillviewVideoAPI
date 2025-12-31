package routers

import (
	"bytes"
	"encoding/json"
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
}

type CloudflareResponse struct {
	Result struct {
		UID                   string  `json:"uid"`
		Thumbnail             string  `json:"thumbnail"`
		ThumbnailTimestampPct float64 `json:"thumbnailTimestampPct"`
		ReadyToStream         bool    `json:"readyToStream"`
		Status                struct {
			State           string `json:"state"`
			ErrorReasonCode string `json:"errorReasonCode"`
			ErrorReasonText string `json:"errorReasonText"`
		} `json:"status"`
		Meta struct {
			DownloadedFrom string `json:"downloaded-from"`
		} `json:"meta"`
		Created            time.Time     `json:"created"`
		Modified           time.Time     `json:"modified"`
		Size               int           `json:"size"`
		Preview            string        `json:"preview"`
		AllowedOrigins     []interface{} `json:"allowedOrigins"`
		RequireSignedURLs  bool          `json:"requireSignedURLs"`
		Uploaded           time.Time     `json:"uploaded"`
		UploadExpiry       interface{}   `json:"uploadExpiry"`
		MaxSizeBytes       interface{}   `json:"maxSizeBytes"`
		MaxDurationSeconds interface{}   `json:"maxDurationSeconds"`
		Duration           int           `json:"duration"`
		Input              struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"input"`
		Playback struct {
			Hls  string `json:"hls"`
			Dash string `json:"dash"`
		} `json:"playback"`
		Watermark interface{} `json:"watermark"`
	} `json:"result"`
	Success  bool          `json:"success"`
	Errors   []interface{} `json:"errors"`
	Messages []interface{} `json:"messages"`
}

type CloudflareRequest struct {
	URL                   string  `json:"url"`
	Name                  string  `json:"name"`
	ThumbnailTimestampPct float64 `json:"thumbnailTimestampPct"`
}

func HandleVideoUpload(w http.ResponseWriter, r *http.Request) {

	resetMultipart := func(w http.ResponseWriter) {
		files, err := os.ReadDir("/tmp")
		if err != nil {
			responder.SendError(w, http.StatusBadRequest, "failed to clear tmp: "+err.Error())
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
		responder.SendError(w, http.StatusBadRequest, "failed to get subject: "+err.Error())
		resetMultipart(w)
		return
	}

	// Get the file from the form data
	file, fileHeader, err := r.FormFile("upload")
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to get formfile: "+err.Error())
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
		responder.SendError(w, http.StatusBadRequest, "failed to upload to s3: "+err.Error())
		resetMultipart(w)
		return
	}

	file.Close()

	log.Println("Successfully uploaded a video to S3:", *response.Location)

	// Create the post body for cloudflare
	postBody, err := json.Marshal(CloudflareRequest{
		URL:                   "https://content.hillview.tv/videos/uploads/" + generated,
		Name:                  strings.TrimSuffix(generated, ".mp4"),
		ThumbnailTimestampPct: 0.5,
	})
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to marshal the post body: "+err.Error())
		resetMultipart(w)
		return
	}
	responseBody := bytes.NewBuffer(postBody)

	// Post the video to cloudflare
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/accounts/"+env.CloudflareUID+"/stream/copy", responseBody)
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to create post to cloudflare: "+err.Error())
		resetMultipart(w)
		return
	}
	req.Header.Set("X-Auth-Email", env.CloudflareEmail)
	req.Header.Set("X-Auth-Key", env.CloudflareKey)
	res, err := client.Do(req)
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to execute post to cloudflare: "+err.Error())
		resetMultipart(w)
		return
	}

	defer res.Body.Close()

	// parse the response from cloudflare
	body := CloudflareResponse{}
	err = json.NewDecoder(res.Body).Decode(&body)
	if err != nil {
		responder.SendError(w, http.StatusBadRequest, "failed to decode body from cloudflare: "+err.Error())
		resetMultipart(w)
		return
	}

	if !body.Success {
		responder.SendError(w, http.StatusBadRequest, "something went wrong in the cloudflare response: "+body.Errors[0].(string))
		resetMultipart(w)
		return
	}

	resetMultipart(w)

	// return the url of the uploaded file
	responder.New(w, VideoUploadResponse{
		URL:       body.Result.Playback.Hls,
		Thumbnail: body.Result.Thumbnail,
		S3Url:     "https://content.hillview.tv/videos/uploads/" + generated,
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
		responder.SendError(w, http.StatusBadRequest, "failed to get subject: "+err.Error())
		return
	}

	sessionHandler := awsBridge.Connect()
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
		Bucket:      aws.String("content.hillview.tv"),
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
		URL: "https://content.hillview.tv/thumbnails/" + generated,
	}, "Thumbnail uploaded successfully")

}

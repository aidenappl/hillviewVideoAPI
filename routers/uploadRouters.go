package routers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hillview.tv/videoAPI/actions"
	"github.com/hillview.tv/videoAPI/awsBridge"
	"github.com/hillview.tv/videoAPI/env"
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
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
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

	// Get the claims from the context
	claims := middleware.WithClaimsValue(r.Context())
	if claims == nil {
		http.Error(w, "Missing Authorization token", http.StatusUnauthorized)
		return
	}

	// Convert the subject to an int
	sub, err := strconv.Atoi(claims.Subject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the multipart form data
	err = r.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		http.Error(w, ""+err.Error(), http.StatusBadRequest)
		return
	}

	// Get the file from the form data
	file, _, err := r.FormFile("upload")
	if err != nil {
		http.Error(w, "failed to get formfile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Create a temporary file to store the uploaded data with the title of upload-[current timestamp]
	tempFile, err := os.CreateTemp("", "upload-"+strconv.FormatInt(time.Now().Unix(), 10)+"-*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	// Copy the uploaded data to the temporary file
	fileSize, err := io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, "failed to copy over file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("Successfully uploaded a video to the local Server (bytes):", fileSize)

	// Reset the file pointer to the start of the file
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate a unique name for the file
	now := time.Now()
	sec := now.Unix()
	generated := "UID" + strconv.Itoa(sub) + "-" + strconv.FormatInt(sec, 10) + "-" + RandStringBytesMaskImpr(10) + ".mp4"

	// Upload the temporary file to s3
	err = actions.UploadFile(tempFile, generated, fileSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// delete the temporary file
	err = os.Remove(tempFile.Name())
	if err != nil {
		http.Error(w, "failed to delete the temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create the post body for cloudflare
	postBody, err := json.Marshal(CloudflareRequest{
		URL:                   "https://content.hillview.tv/videos/uploads/" + generated,
		Name:                  strings.TrimSuffix(generated, ".mp4"),
		ThumbnailTimestampPct: 0.5,
	})
	if err != nil {
		http.Error(w, "failed to marshal the post body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	responseBody := bytes.NewBuffer(postBody)

	// Post the video to cloudflare
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/accounts/"+env.CloudflareUID+"/stream/copy", responseBody)
	if err != nil {
		http.Error(w, "failed to create post to cloudflare: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("X-Auth-Email", env.CloudflareEmail)
	req.Header.Set("X-Auth-Key", env.CloudflareKey)
	res, err := client.Do(req)
	if err != nil {
		http.Error(w, "failed to execute post to cloudflare: "+err.Error(), http.StatusInternalServerError)
		return
	}

	defer res.Body.Close()

	// parse the response from cloudflare
	body := CloudflareResponse{}
	err = json.NewDecoder(res.Body).Decode(&body)
	if err != nil {
		http.Error(w, "failed to decode body from cloudflare: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !body.Success {
		http.Error(w, "something went wrong in the cloudflare response: "+body.Errors[0].(string), http.StatusInternalServerError)
		return
	}

	//return the url of the uploaded file
	json.NewEncoder(w).Encode(VideoUplaodResponse{
		URL:       body.Result.Playback.Hls,
		Thumbnail: body.Result.Thumbnail,
	})
}

type ThumbnailUploadResponse struct {
	URL string `json:"url"`
}

func HandleThumbnailUpload(w http.ResponseWriter, r *http.Request) {

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(ThumbnailUploadResponse{
		URL: "https://content.hillview.tv/thumbnails/" + generated,
	})

}

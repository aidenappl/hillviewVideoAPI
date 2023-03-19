package actions

import (
	"bytes"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	BUCKET_NAME = "content.hillview.tv"
	REGION      = "us-west-1"

	PART_SIZE = 33_554_432 // Has to be 5_000_000 minimim
	RETRIES   = 2
)

var (
	s3session *s3.S3
)

func init() {
	s3session = s3.New(session.Must(session.NewSession(&aws.Config{
		Region: aws.String(REGION),
	})))
}

func UploadFile(file *os.File, key string, fileSize int64) error {
	// Open the file
	defer file.Close()

	// put file in byteArray
	buffer := make([]byte, fileSize)
	file.Read(buffer)

	// Create MultipartUpload object
	createdResp, err := s3session.CreateMultipartUpload(&s3.CreateMultipartUploadInput{
		Bucket:      aws.String(BUCKET_NAME),
		Key:         aws.String("videos/uploads/" + *aws.String(key)),
		ContentType: aws.String("video/mp4"),
		ACL:         aws.String("public-read"),
	})

	if err != nil {
		return err
	}

	var start, currentSize int
	var remaining = int(fileSize)
	var partNum = 1
	var completedParts []*s3.CompletedPart
	// Loop till remaining upload size is 0
	for start = 0; remaining != 0; start += PART_SIZE {
		if remaining < PART_SIZE {
			currentSize = remaining
		} else {
			currentSize = PART_SIZE
		}

		completed, err := Upload(createdResp, buffer[start:start+currentSize], partNum)
		// If upload function failed (meaning it retried acoording to RETRIES)
		if err != nil {
			_, err = s3session.AbortMultipartUpload(&s3.AbortMultipartUploadInput{
				Bucket:   createdResp.Bucket,
				Key:      createdResp.Key,
				UploadId: createdResp.UploadId,
			})
			if err != nil {
				return err
			}
		}

		// Detract the current part size from remaining
		remaining -= currentSize
		fmt.Printf("Part %v complete, %v btyes remaining\n", partNum, remaining)

		// Add the completed part to our list
		completedParts = append(completedParts, completed)
		partNum++

	}

	// All the parts are uploaded, completing the upload
	_, err = s3session.CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
		Bucket:   createdResp.Bucket,
		Key:      createdResp.Key,
		UploadId: createdResp.UploadId,
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// Uploads the fileBytes bytearray a MultiPart upload
func Upload(resp *s3.CreateMultipartUploadOutput, fileBytes []byte, partNum int) (completedPart *s3.CompletedPart, err error) {
	var try int
	for try <= RETRIES {
		uploadResp, err := s3session.UploadPart(&s3.UploadPartInput{
			Body:          bytes.NewReader(fileBytes),
			Bucket:        resp.Bucket,
			Key:           resp.Key,
			PartNumber:    aws.Int64(int64(partNum)),
			UploadId:      resp.UploadId,
			ContentLength: aws.Int64(int64(len(fileBytes))),
		})
		// Upload failed
		if err != nil {
			fmt.Println(err)
			// Max retries reached! Quitting
			if try == RETRIES {
				return nil, err
			} else {
				// Retrying
				try++
			}
		} else {
			// Upload is done!
			return &s3.CompletedPart{
				ETag:       uploadResp.ETag,
				PartNumber: aws.Int64(int64(partNum)),
			}, nil
		}
	}

	return nil, nil
}

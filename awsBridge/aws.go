package awsBridge

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hillview.tv/videoAPI/env"
)

// ContentSession is the session for the content store (OpenBucket at
// CONTENT_S3_ENDPOINT, bucket CONTENT_S3_BUCKET): thumbnail uploads and
// presigned video downloads. Path-style, because the bucket is addressed by
// path on a single host.
func ContentSession() *session.Session {
	sess, err := session.NewSession(
		&aws.Config{
			Region:           aws.String(env.ContentS3Region),
			Endpoint:         aws.String(env.ContentS3Endpoint),
			S3ForcePathStyle: aws.Bool(true),
			Credentials: credentials.NewStaticCredentials(
				env.ContentS3AccessKeyID,
				env.ContentS3SecretAccessKey,
				"",
			),
		})
	if err != nil {
		panic(err)
	}
	return sess
}

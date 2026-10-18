package awsBridge

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hillview.tv/videoAPI/env"
)

var AccessKeyID string
var SecretAccessKey string
var MyRegion string

func Connect() *session.Session {
	AccessKeyID = env.AWSKey
	SecretAccessKey = env.AWSSecret
	MyRegion = env.AWSRegion
	sess, err := session.NewSession(
		&aws.Config{
			Region: aws.String(MyRegion),
			Credentials: credentials.NewStaticCredentials(
				AccessKeyID,
				SecretAccessKey,
				"", // a token will be created when the session it's used.
			),
		})
	if err != nil {
		panic(err)
	}
	return sess
}

// ContentSession is the session for thumbnail uploads: the S3-compatible
// content store (OpenBucket) when CONTENT_S3_ENDPOINT is set, else AWS.
func ContentSession() *session.Session {
	if env.ContentS3Endpoint == "" {
		return Connect()
	}
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

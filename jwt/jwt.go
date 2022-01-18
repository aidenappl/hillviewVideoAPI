package jwt

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/hillview.tv/videoAPI/env"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
)

var (
	JWTExpiresIn = 3600
	AccessToken  = "access_token"
	RefreshToken = "refresh_token"
)

type HVJwtClaims struct {
	Type string `json:"typ"`
	jwt.StandardClaims
}

type HVResponseJWT struct {
	userID int
	Type   string `json:"typ"`
	jwt.StandardClaims
}

type ValidTokenResponse struct {
	Expired       bool
	Invalid       bool
	Revoked       bool
	InvalidIssuer bool
	Err           bool
}

func ValidJWT(ctx context.Context, rawToken string, c *HVJwtClaims, reqClaims *HVJwtClaims) (bool, ValidTokenResponse, error) {
	if reqClaims == nil {
		reqClaims = &HVJwtClaims{}
	}

	if len(reqClaims.Type) > 0 {
		if c.Type != reqClaims.Type {
			log.Println("invalid token type")
			return false, ValidTokenResponse{Invalid: true}, nil
		}
	} else {
		if c.Type != "access_token" && c.Type != "refresh_token" {
			log.Println("invalid token type does not match")
			return false, ValidTokenResponse{Invalid: true}, nil
		}
	}

	if len(reqClaims.Issuer) > 0 {
		if c.Issuer != reqClaims.Issuer {
			log.Println("bad issuer")
			return false, ValidTokenResponse{InvalidIssuer: true}, nil
		}
	} else {
		if c.Issuer != "hillview:auth-service" {
			log.Println("bad issuer")
			return false, ValidTokenResponse{InvalidIssuer: true}, nil
		}
	}

	// TODO - check if the token is expired or revoked

	// redisClient, err := db.NewRedisClient()
	// if err != nil {
	// 	return false, ValidTokenResponse{Err: true}, fmt.Errorf("error creating redis client: %w", err)
	// }
	// rts, err := db.GetTokenRevocation(redisClient, rawToken)
	// if err != nil {
	// 	return false, ValidTokenResponse{Err: true}, fmt.Errorf("error getting user %d's token revocation time: %w", userID, err)
	// }
	// redisClient.Close()

	// if rts == nil {
	// 	return true, ValidTokenResponse{}, nil
	// }

	// rt, err := strconv.ParseInt(*rts, 10, 64)
	// if err != nil {
	// 	return false, ValidTokenResponse{Err: true}, fmt.Errorf("error converting token revocation time to int64: %w", err)
	// }

	// if rt > c.IssuedAt {
	// 	// token has been revoked
	// 	return false, ValidTokenResponse{Revoked: true}, nil
	// }

	return true, ValidTokenResponse{}, nil
}

func newJWT() (*jwt.Token, error) {
	jti, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("error generating uuid for jti: %w", err)
	}
	claims := &HVJwtClaims{
		StandardClaims: jwt.StandardClaims{
			Audience: "",
			Id:       jti.String(),
			IssuedAt: time.Now().Unix(),
			Issuer:   "hillview:auth-service",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)

	return token, nil
}

func NewRefreshJWT(userID int) (string, error) {
	token, err := newJWT()
	if err != nil {
		return "", fmt.Errorf("error generating base refresh jwt: %w", err)
	}

	claims := token.Claims.(*HVJwtClaims)
	claims.Type = "refresh_token"
	claims.Subject = strconv.Itoa(userID)

	token.Claims = claims

	ss, err := token.SignedString([]byte(env.JWTSigningKey))
	if err != nil {
		return "", fmt.Errorf("error generating refresh jwt: %w", err)
	}

	return ss, nil
}

func NewAccessJWT(userID int) (string, error) {
	token, err := newJWT()
	if err != nil {
		return "", fmt.Errorf("error generating base access jwt: %w", err)
	}

	claims := token.Claims.(*HVJwtClaims)
	claims.Type = "access_token"
	claims.Subject = strconv.Itoa(userID)
	claims.ExpiresAt = claims.IssuedAt + int64(JWTExpiresIn)

	token.Claims = claims

	ss, err := token.SignedString([]byte(env.JWTSigningKey))
	if err != nil {
		return "", fmt.Errorf("error generating access jwt: %w", err)
	}

	return ss, nil
}

/*
ParseJWT ensures that a given JWT is signed by Hillview and is still valid.
*/
func ParseJWT(rawToken string) (*jwt.Token, error) {
	return jwt.ParseWithClaims(rawToken, &HVJwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(env.JWTSigningKey), nil
	})
}

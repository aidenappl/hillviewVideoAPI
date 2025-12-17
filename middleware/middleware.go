package middleware

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hillview.tv/videoAPI/db"
	"github.com/hillview.tv/videoAPI/jwt"
	"github.com/hillview.tv/videoAPI/query"
	"github.com/hillview.tv/videoAPI/responder"
	"github.com/hillview.tv/videoAPI/structs"
)

type contextKey struct {
	name string
}

var JWTClaimsCtxKey = &contextKey{"jwt_claims"}
var RequestIDKey = &contextKey{"request_id"}
var ClientIPKey = &contextKey{"client_ip"}
var UserModelCtxKey = &contextKey{"user_model"}

func WithClaimsValue(ctx context.Context) *jwt.HVJwtClaims {
	val, ok := ctx.Value(JWTClaimsCtxKey).(*jwt.HVJwtClaims)
	if !ok {
		return nil
	}

	return val
}

func WithUserModelValue(ctx context.Context) *structs.User {
	val, ok := ctx.Value(UserModelCtxKey).(*structs.User)
	if !ok {
		return nil
	}

	return val
}

// GetClientIP extracts the real client IP from the request,
// checking common proxy headers first
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (common for proxies/load balancers)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header (used by nginx)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Check CF-Connecting-IP header (Cloudflare)
	if cfip := r.Header.Get("CF-Connecting-IP"); cfip != "" {
		return cfip
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// GetClientIPFromContext retrieves the client IP from the context
func GetClientIPFromContext(ctx context.Context) string {
	if ip, ok := ctx.Value(ClientIPKey).(string); ok {
		return ip
	}
	return "unknown"
}

// RequestIDMiddleware generates a unique request ID and adds it to the context
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		clientIP := GetClientIP(r)
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		ctx = context.WithValue(ctx, ClientIPKey, clientIP)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Println(ctx context.Context, msg string) {
	requestID := GetRequestID(ctx)
	log.Printf("[%s] %s", requestID, msg)
}

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return "unknown"
}

// LoggingMiddleware logs the request method, URI, and duration with request ID and client IP
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := GetRequestID(r.Context())
		clientIP := GetClientIPFromContext(r.Context())
		log.Printf("[%s] [%s] %s %s", requestID, clientIP, r.Method, r.RequestURI)
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Printf("[%s] [%s] [REQUEST FINISH] %s %s - %v", requestID, clientIP, r.Method, r.RequestURI, duration)
	})
}

func MuxHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, "+
			"Content-Type, "+
			"Accept-Encoding, "+
			"Connection, "+
			"Content-Length")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Server", "Go")
		next.ServeHTTP(w, r)
	})
}

func TokenHandlers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// get token from header
		rawToken := r.Header.Get("Authorization")
		splitToken := strings.Split(rawToken, "Bearer ")

		next.ServeHTTP(w, r)

		if len(splitToken) != 2 {
			return
		}
		rawToken = splitToken[1]

		if len(rawToken) < 1 {
			return
		}

		// parse token validity
		token, err := jwt.ParseJWT(rawToken)
		if err != nil {
			return
		}

		claims := token.Claims.(*jwt.HVJwtClaims)

		userID, err := strconv.Atoi(claims.Subject)
		if err != nil {
			return
		}

		if userID != 0 && len(r.RequestURI) != 0 && len(r.Method) != 0 {
			err = query.InsertRequestLog(db.DB, userID, r.RequestURI, r.Method)
			if err != nil {
				log.Println(fmt.Errorf("failed to insert request log: %w", err))
			}
		}

	})
}

func AccessTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// get token from header
		rawToken := r.Header.Get("Authorization")
		if rawToken == "" {
			uploadRaw := r.Header.Get("Cookies")
			if uploadRaw != "" {
				rawToken = uploadRaw
			}
		}
		splitToken := strings.Split(rawToken, "Bearer ")

		if len(splitToken) != 2 {
			responder.SendError(w, http.StatusUnauthorized, "Missing Authorization token")
			return
		}
		rawToken = splitToken[1]

		if len(rawToken) < 1 {
			responder.SendError(w, http.StatusUnauthorized, "Missing Authorization token")
			return
		}

		// parse token validity
		token, err := jwt.ParseJWT(rawToken)
		if err != nil {
			if strings.Contains(err.Error(), "token is expired") {
				responder.SendError(w, http.StatusUnauthorized, "Token is expired")
			} else {
				responder.SendError(w, http.StatusUnauthorized, "failed to parse token", err)
			}
			return
		}

		claims := token.Claims.(*jwt.HVJwtClaims)

		claimsValid, resp, err := jwt.ValidJWT(r.Context(), rawToken, claims, &jwt.HVJwtClaims{Type: jwt.AccessToken})
		if err != nil {
			responder.SendError(w, http.StatusUnauthorized, "failed to parse token", err)
			return
		}

		if !claimsValid {
			if resp.Expired {
				responder.SendError(w, http.StatusUnauthorized, "Token is expired")
			}
			if resp.Revoked {
				responder.SendError(w, http.StatusUnauthorized, "Token is revoked")
			}
			if resp.InvalidIssuer || resp.Err || resp.Invalid {
				responder.SendError(w, http.StatusUnauthorized, "Invalid token, bad issuer, response, or invalid")
			}
			return
		}

		userID, err := strconv.Atoi(claims.Subject)
		if err != nil {
			log.Println("failed to convert user id to int", err.Error())
			responder.SendError(w, http.StatusUnauthorized, "Invalid token", err)
			return
		}

		user, err := query.FindUser(db.DB, query.FindUserRequest{ID: &userID})
		if err != nil {
			log.Println("failed to find user by id", err.Error())
			responder.SendError(w, http.StatusUnauthorized, "Invalid token", err)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, UserModelCtxKey, user)
		ctx = context.WithValue(ctx, JWTClaimsCtxKey, claims)

		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

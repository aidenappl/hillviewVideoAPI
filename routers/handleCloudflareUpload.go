package routers

import (
	"fmt"
	"net/http"

	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/responder"
)

func HandleCloudflareUpload(w http.ResponseWriter, r *http.Request) {

	cloudflareAccountID := env.CloudflareUID
	cloudflareAPIToken := env.CloudflareToken
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream?direct_user=true", cloudflareAccountID)

	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "Failed to create cloudflare request", err)
		return
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+cloudflareAPIToken)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", r.Header.Get("Upload-Length"))
	req.Header.Set("Upload-Metadata", r.Header.Get("Upload-Metadata"))
	req.Header.Set("Origin", r.Header.Get("Origin"))

	resp, err := client.Do(req)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "Failed to do cloudflare request", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		responder.SendError(w, http.StatusRequestEntityTooLarge, "Out of storage", nil)
		return
	}

	if resp.StatusCode != http.StatusCreated {
		responder.SendError(w, http.StatusNotAcceptable, "Error code from Cloudflare", nil)
		return
	}

	location := resp.Header.Get("Location")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Location")
	w.Header().Set("Location", location)
	responder.New(w, map[string]string{"location": location})

}

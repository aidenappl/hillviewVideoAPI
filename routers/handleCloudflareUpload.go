package routers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hillview.tv/videoAPI/env"
)

func HandleCloudflareUpload(w http.ResponseWriter, r *http.Request) {

	cloudflareAccountID := env.CloudflareUID
	cloudflareAPIToken := env.CloudflareToken
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream?direct_user=true", cloudflareAccountID)

	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		http.Error(w, "Failed to create cloudflare request: "+err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "Failed to do cloudflare request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		http.Error(w, "Out of storage", http.StatusRequestEntityTooLarge)
		return
	}

	if resp.StatusCode != http.StatusCreated {
		http.Error(w, "Error code from Cloudflare", http.StatusNotAcceptable)
		return
	}

	location := resp.Header.Get("Location")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Location")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"location": location})

}

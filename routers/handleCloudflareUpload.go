package routers

import (
	"encoding/json"
	"fmt"
	"io"
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
		fmt.Println("Failed to start upload video to cloudflare: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+cloudflareAPIToken)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", r.Header.Get("Upload-Length"))
	req.Header.Set("Upload-Metadata", r.Header.Get("Upload-Metadata"))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Failed to start upload video to cloudflare: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		fmt.Println("Failed to start upload video to cloudflare: %v", resp)
		json.NewEncoder(w).Encode(resp)
		http.Error(w, "Failed to start upload video to cloudflare", http.StatusNotAcceptable)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to start upload video to cloudflare: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println("Failed to start upload video to cloudflare: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	location := resp.Header.Get("Location")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Location")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"location": location})

}

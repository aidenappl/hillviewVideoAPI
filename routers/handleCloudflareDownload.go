package routers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/responder"
)

func HandleCloudflareDownload(w http.ResponseWriter, r *http.Request) {

	fmt.Println("HandleCloudflareStatus")

	params := mux.Vars(r)
	id := params["id"]

	if len(id) == 0 {
		responder.ErrMissingBodyRequirement(w, "id")
		return
	}

	cloudflareAccountID := env.CloudflareUID
	cloudflareAPIToken := env.CloudflareToken
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream/%s/downloads", cloudflareAccountID, id)

	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "Failed to create request", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+cloudflareAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "Failed to fetch video status", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "Failed to read response body", err)
		return
	}

	// Forward the response from Cloudflare
	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	// Parse the JSON response from Cloudflare
	var cloudflareResponse map[string]interface{}
	if err := json.Unmarshal(body, &cloudflareResponse); err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to parse Cloudflare response", err)
		return
	}

	responder.New(w, cloudflareResponse, "Cloudflare download initiated successfully")
}

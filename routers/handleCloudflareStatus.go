package routers

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/env"
	"github.com/hillview.tv/videoAPI/responder"
)

func HandleCloudflareStatus(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)
	id := params["id"]

	if len(id) == 0 {
		responder.ErrMissingBodyRequirement(w, "id")
		return
	}

	cloudflareAccountID := env.CloudflareUID
	cloudflareAPIToken := env.CloudflareToken
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream/%s", cloudflareAccountID, id)

	client := &http.Client{}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to create request", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+cloudflareAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to fetch video status", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		responder.SendError(w, http.StatusInternalServerError, "failed to read response body", err)
		return
	}

	// Forward the response from Cloudflare
	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}
	responder.New(w, body, "Cloudflare status retrieved successfully")
}

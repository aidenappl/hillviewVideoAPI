package routers

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hillview.tv/videoAPI/env"
)

func HandleCloudflareUpdate(w http.ResponseWriter, r *http.Request) {

	fmt.Println("HandleCloudflareStatus")

	params := mux.Vars(r)
	id := params["id"]

	if len(id) == 0 {
		http.Error(w, "missing id param", http.StatusBadRequest)
		return
	}

	cloudflareAccountID := env.CloudflareUID
	cloudflareAPIToken := env.CloudflareToken
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/stream/%s", cloudflareAccountID, id)

	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		http.Error(w, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header.Set("Authorization", "Bearer "+cloudflareAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to fetch video status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward the response from Cloudflare
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

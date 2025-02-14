package routers

import "net/http"

func HandleSpotlightList(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

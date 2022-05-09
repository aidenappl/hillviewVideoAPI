package structs

type Playlist struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	BannerImage string   `json:"banner_image"`
	Route       string   `json:"route"`
	InsertedAt  string   `json:"inserted_at"`
	Videos      []*Video `json:"videos"`
}

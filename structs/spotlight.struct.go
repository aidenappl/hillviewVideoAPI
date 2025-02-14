package structs

type Spotlight struct {
	Rank       int    `json:"rank"`
	VideoID    *int   `json:"video_id"`
	InsertedAt string `json:"inserted_at"`
	UpdatedAt  string `json:"updated_at"`

	// Additional fields
	Video *Video `json:"video"`
}

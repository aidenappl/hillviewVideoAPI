package structs

type Spotlight struct {
	Position   int    `json:"position"`
	VideoID    *int   `json:"video_id"`
	InsertedAt string `json:"inserted_at"`
	UpdatedAt  string `json:"updated_at"`

	// Additional fields
	Video *Video `json:"video"`
}

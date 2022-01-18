package structs

type MobileUser struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Identifier      string `json:"identifier"`
	ProfileImageURL string `json:"profile_image_url"`
	InsertedAt      string `json:"inserted_at"`
}

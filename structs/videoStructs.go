package structs

type Video struct {
	ID          int         `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Thumbnail   *string     `json:"thumbnail"`
	URL         string      `json:"url"`
	Status      *GeneralNSN `json:"status"`
	InsertedAt  string      `json:"inserted_at"`
}

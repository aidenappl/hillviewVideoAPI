package structs

type GeneralNSN struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

type GeneralNSNNulled struct {
	ID        *int    `json:"id"`
	Name      *string `json:"name"`
	ShortName *string `json:"short_name"`
}

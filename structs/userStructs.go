package structs

import "time"

type User struct {
	ID                       int                           `json:"id"`
	Username                 *string                       `json:"username"`
	Name                     string                        `json:"name"`
	Email                    string                        `json:"email"`
	ProfileImageURL          string                        `json:"profile_image_url"`
	Authentication           GeneralNST                    `json:"authentication"`
	InsertedAt               time.Time                     `json:"inserted_at"`
	LastActive               *time.Time                    `json:"last_active"`
	AuthenticationStrategies *UserAuthenticationStrategies `json:"strategies,omitempty"`
}

type UserAuthenticationStrategies struct {
	UserID   *int    `json:"user_id,omitempty"`
	GoogleID *string `json:"google_id"`
	Password *string `json:"password"`
}

type GeneralNST struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

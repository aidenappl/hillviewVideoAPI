package structs

import "time"

type Asset struct {
	ID          int64          `json:"id"`
	Name        *string        `json:"name"`
	ImageURL    *string        `json:"image_url"`
	Identifier  *string        `json:"identifier"`
	Description *string        `json:"description"`
	Category    *GeneralNSN    `json:"category"`
	Status      *GeneralNSN    `json:"status"`
	Metadata    *AssetMetadata `json:"metadata"`
	ActiveTab   *AssetCheckout `json:"active_tab"`
}

type AssetCheckout struct {
	ID             int         `json:"id"`
	AssetID        int         `json:"asset_id"`
	CheckoutStatus *GeneralNSN `json:"checkout_status"`
	AssociatedUser *int        `json:"associated_user"`
	CheckoutNotes  *string     `json:"checkout_notes"`
	TimeOut        *time.Time  `json:"time_out"`
	TimeIn         *time.Time  `json:"time_in"`
	ExpectedIn     *time.Time  `json:"expected_in"`
	User           *MobileUser `json:"user"`
}

type AssetMetadata struct {
	SerialNumber *string `json:"serial_number"`
	Manufacturer *string `json:"manufacturer"`
	Model        *string `json:"model"`
	Notes        *string `json:"notes"`
}

type GeneralNSN struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

type Checkout struct {
	ID             int         `json:"id"`
	User           *MobileUser `json:"user"`
	AssociatedUser *int        `json:"associated_user,omitempty"`
	Asset          *Asset      `json:"asset"`
	AssetID        *int        `json:"asset_id,omitempty"`
	Offsite        int         `json:"offsite"`
	CheckoutStatus *GeneralNSN `json:"checkout_status"`
	CheckoutNotes  *string     `json:"checkout_notes"`
	TimeOut        *time.Time  `json:"time_out"`
	TimeIn         *time.Time  `json:"time_in"`
	ExpectedIn     *time.Time  `json:"expected_in"`
}

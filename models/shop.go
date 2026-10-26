package models

type Shop struct {
	ID       int64  `json:"id"`
	OwnerID  int64  `json:"owner_id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Location string `json:"location"`
}

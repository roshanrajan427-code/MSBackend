package models

type Product struct {
	ID       int64   `json:"id"`
	ShopID   int64   `json:"shop_id"`
	Name     string  `json:"name"`
	Stock    int     `json:"stock"`
	Price    float64 `json:"price"`
	Color    string  `json:"color"`
	Size     string  `json:"size"`
	ImageURL string  `json:"image_url"`
}

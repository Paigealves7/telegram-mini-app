package domain

type FirewoodProcessing struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind"`
	VolumeM3    float64 `json:"volume_m3"`
	Coefficient float64 `json:"coefficient"`
	RawM3       float64 `json:"raw_m3"`
	Performer   string  `json:"performer"`
	Date        string  `json:"date"`
	Comment     string  `json:"comment"`
}

type FirewoodOrderItem struct {
	Kind     string  `json:"kind" validate:"oneof=split round"`
	VolumeM3 float64 `json:"volume_m3" validate:"gt=0,lte=100000"`
	Price    float64 `json:"price" validate:"gte=0,lte=100000000"`
}

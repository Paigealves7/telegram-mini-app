package domain

// OrderLine — позиция заказа вместе с описанием доски со склада (для накладной).
// Price — цена за 1 м³.
type OrderLine struct {
	BoardID  int64
	Species  string
	Grade    string
	HeightMM int64
	WidthMM  int64
	LengthMM int64
	Count    int64
	VolumeM3 float64
	Price    float64
}

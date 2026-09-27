package domain

import "time"

// Contractor — Контрагент (Покупатель или Поставщик) из Раздела 3 ТЗ
type Contractor struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Role string

const (
	RoleManager Role = "manager"
	RoleMaster  Role = "master"
	RoleWorker  Role = "worker"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // Игнорируем при сериализации в JSON
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// LogItem — Бревно (из Раздела 3 ТЗ)
type LogItem struct {
	ID         int64   `json:"id"`
	ArrivalID  int64   `json:"arrival_id"`
	LengthMM   int     `json:"length_mm"`
	DiameterMM int     `json:"diameter_mm"`
	Species    string  `json:"species"`
	Count      int     `json:"count"`
	VolumeM3   float64 `json:"volume_m3"`
}

// LogArrival — Поступление круглого леса (из Раздела 3 ТЗ)
type LogArrival struct {
	ID          int64     `json:"id"`
	SupplierID  int64     `json:"supplier_id"`
	CarrierID   int64     `json:"carrier_id"`
	TruckNumber string    `json:"truck_number"`
	ArrivalDate string    `json:"arrival_date"` // YYYY-MM-DD
	CreatedBy   int64     `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	Items       []LogItem `json:"items,omitempty"`
}

type SawedLog struct {
	ID       int64   `json:"id"`
	SawingID int64   `json:"sawing_id"`
	LogID    *int64  `json:"log_id,omitempty"`
	VolumeM3 float64 `json:"volume_m3"`
}

// Board — Полученная доска (Раздел 3 ТЗ)
type Board struct {
	ID         int64    `json:"id"`
	HeightMM   int      `json:"height_mm"`
	WidthMM    int      `json:"width_mm"`
	LengthMM   int      `json:"length_mm"`
	Species    string   `json:"species"`
	Grade      string   `json:"grade"`
	Count      int      `json:"count"`
	VolumeM3   float64  `json:"volume_m3"`
	PricePerM3 *float64 `json:"price_per_m3,omitempty"`
}

// SawingOperation — Операция распила (Раздел 3 ТЗ)
type SawingOperation struct {
	ID        int64      `json:"id"`
	WorkerID  int64      `json:"worker_id"`
	Date      string     `json:"date"` // YYYY-MM-DD
	CreatedAt time.Time  `json:"created_at"`
	Logs      []SawedLog `json:"logs,omitempty"`
	Boards    []Board    `json:"boards,omitempty"`
}

// SaleItem — Позиция в чеке/накладной продажи (Раздел 3 ТЗ)
type SaleItem struct {
	ID       int64   `json:"id"`
	SaleID   int64   `json:"sale_id"`
	BoardID  int64   `json:"board_id"`
	Count    int     `json:"count"`
	Price    float64 `json:"price"`
	VolumeM3 float64 `json:"volume_m3"`
}

// Sale — Накладная продажи (Раздел 3 ТЗ)
type Sale struct {
	ID          int64      `json:"id"`
	BuyerID     int64      `json:"buyer_id"`
	TotalAmount float64    `json:"total_amount"`
	SaleDate    string     `json:"sale_date"` // YYYY-MM-DD
	CreatedBy   int64      `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	Items       []SaleItem `json:"items,omitempty"`
}

type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "new"
	OrderStatusInProgress OrderStatus = "in_progress"
	OrderStatusCompleted  OrderStatus = "completed"
	OrderStatusCanceled   OrderStatus = "canceled"
)

// OrderItem — позиция заказа (ссылается на доску из таблицы boards)
type OrderItem struct {
	ID       int64   `json:"id"`
	OrderID  int64   `json:"order_id"`
	BoardID  int64   `json:"board_id"`
	Count    int     `json:"count"`
	VolumeM3 float64 `json:"volume_m3"`
	Price    float64 `json:"price"`
}

// Order — заказ покупателя
type Order struct {
	ID            int64       `json:"id"`
	MasterID      int64       `json:"master_id"`
	ContractorID  int64       `json:"contractor_id"`
	Notes         string      `json:"notes,omitempty"`
	DeliveryPrice float64     `json:"delivery_price"`
	ExtraPrice    float64     `json:"extra_price"`
	Status        OrderStatus `json:"status"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Items         []OrderItem `json:"items,omitempty"`
}

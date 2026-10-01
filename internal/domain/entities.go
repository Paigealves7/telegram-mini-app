package domain

import "time"

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
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type LogItem struct {
	ID         int64   `json:"id"`
	ArrivalID  int64   `json:"arrival_id"`
	LengthMM   int     `json:"length_mm" validate:"required,gt=0"`
	DiameterMM int     `json:"diameter_mm" validate:"required,gt=0"`
	Species    string  `json:"species" validate:"required"`
	Count      int     `json:"count" validate:"required,gt=0"`
	VolumeM3   float64 `json:"volume_m3" validate:"required,gt=0"`
}

type LogArrival struct {
	ID          int64     `json:"id"`
	SupplierID  int64     `json:"supplier_id"`
	CarrierID   int64     `json:"carrier_id"`
	TruckNumber string    `json:"truck_number"`
	ArrivalDate string    `json:"arrival_date"`
	CreatedBy   int64     `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	Items       []LogItem `json:"items,omitempty"`
}

type SawedLog struct {
	ID       int64   `json:"id"`
	SawingID int64   `json:"sawing_id"`
	LogID    *int64  `json:"log_id,omitempty" validate:"required"`
	VolumeM3 float64 `json:"volume_m3" validate:"required,gt=0"`
}

type Board struct {
	ID         int64    `json:"id"`
	HeightMM   int      `json:"height_mm" validate:"required,gt=0"`
	WidthMM    int      `json:"width_mm" validate:"required,gt=0"`
	LengthMM   int      `json:"length_mm" validate:"required,gt=0"`
	Species    string   `json:"species" validate:"required"`
	Grade      string   `json:"grade"` // Сорт может быть пустым по бизнес-логике
	Count      int      `json:"count" validate:"required,gt=0"`
	VolumeM3   float64  `json:"volume_m3" validate:"required,gt=0"`
	PricePerM3 *float64 `json:"price_per_m3,omitempty" validate:"omitempty,gt=0"`
}

type SawingOperation struct {
	ID        int64      `json:"id"`
	WorkerID  int64      `json:"worker_id"`
	Date      string     `json:"date"`
	CreatedAt time.Time  `json:"created_at"`
	Logs      []SawedLog `json:"logs,omitempty"`
	Boards    []Board    `json:"boards,omitempty"`
}

type SaleItem struct {
	ID       int64   `json:"id"`
	SaleID   int64   `json:"sale_id"`
	BoardID  int64   `json:"board_id" validate:"required,gt=0"`
	Count    int     `json:"count" validate:"required,gt=0"`
	Price    float64 `json:"price" validate:"gte=0"` // Цена может быть 0 (например, бонус/подарок), но не отрицательная
	VolumeM3 float64 `json:"volume_m3" validate:"required,gt=0"`
}

type Sale struct {
	ID          int64      `json:"id"`
	BuyerID     int64      `json:"buyer_id"`
	TotalAmount float64    `json:"total_amount"`
	SaleDate    string     `json:"sale_date"`
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

type OrderItem struct {
	ID       int64   `json:"id"`
	OrderID  int64   `json:"order_id"`
	BoardID  int64   `json:"board_id" validate:"required,gt=0"`
	Count    int     `json:"count" validate:"required,gt=0"`
	VolumeM3 float64 `json:"volume_m3" validate:"required,gt=0"`
	Price    float64 `json:"price" validate:"gte=0"`
}

type Order struct {
	ID            int64       `json:"id"`
	MasterID      int64       `json:"master_id"`
	ContractorID  int64       `json:"contractor_id"`
	Notes         string      `json:"notes,omitempty"`
	DeliveryPrice float64     `json:"delivery_price" validate:"gte=0"`
	ExtraPrice    float64     `json:"extra_price" validate:"gte=0"`
	Status        OrderStatus `json:"status"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Items         []OrderItem `json:"items,omitempty"`
}

type StatisticsSummary struct {
	TotalLogsArrivedVolume float64 `json:"total_logs_arrived_volume"`
	TotalLogsSawedVolume   float64 `json:"total_logs_sawed_volume"`
	TotalSalesVolume       float64 `json:"total_sales_volume"`
	TotalSalesAmount       float64 `json:"total_sales_amount"`
}

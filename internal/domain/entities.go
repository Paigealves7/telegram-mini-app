package domain

import "time"

type Contractor struct {
	ID         int64  `json:"id" db:"id"`
	Name       string `json:"name" db:"name"`
	IsSupplier bool   `json:"is_supplier" db:"is_supplier"`
	IsCarrier  bool   `json:"is_carrier" db:"is_carrier"`
	IsClient   bool   `json:"is_client" db:"is_client"`
	IsActive   bool   `json:"is_active" db:"is_active"`
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
	// Имя и @username из Telegram (заполняются при входе через Telegram; в обычные ответы не попадают).
	DisplayName string `json:"-"`
	TGUsername  string `json:"-"`
}

type LogItem struct {
	ID         int64   `json:"id"`
	ArrivalID  int64   `json:"arrival_id"`
	LengthMM   int     `json:"length_mm" validate:"required,gt=0"`
	DiameterMM int     `json:"diameter_mm" validate:"required,gt=0"`
	Species    string  `json:"species" validate:"required,max=64"`
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
	Species    string   `json:"species" validate:"required,max=64"`
	Grade      string   `json:"grade"`
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
	Price    float64 `json:"price" validate:"gte=0"`
	VolumeM3 float64 `json:"volume_m3"` // считает сервер по складу; значение от клиента игнорируется
}

type Sale struct {
	ID            int64      `json:"id"`
	ContractorID  int64      `json:"contractor_id"`
	TruckNumber   string     `json:"truck_number"`
	DeliveryPrice float64    `json:"delivery_price"`
	ExtraPrice    float64    `json:"extra_price"`
	TotalVolume   float64    `json:"total_volume"`
	TotalPrice    float64    `json:"total_price"`
	CreatedBy     int64      `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	Items         []SaleItem `json:"items,omitempty"`
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
	VolumeM3 float64 `json:"volume_m3"` // считает сервер по складу; значение от клиента игнорируется
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
	// Считаются при чтении списка: сумма заказа (позиции + доставка + допы) и сколько уже оплачено.
	TotalPrice float64 `json:"total_price"`
	PaidAmount float64 `json:"paid_amount"`
}

type Payment struct {
	ID        int64     `json:"id"`
	OrderID   int64     `json:"order_id"`
	Amount    float64   `json:"amount"`
	Note      string    `json:"note"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// FirewoodArrival: приход дров (берёза) общей кубатурой.
type FirewoodArrival struct {
	ID          int64     `json:"id"`
	SupplierID  int64     `json:"supplier_id"`
	CarrierID   int64     `json:"carrier_id"`
	TruckNumber string    `json:"truck_number"`
	VolumeM3    float64   `json:"volume_m3"`
	ArrivalDate string    `json:"arrival_date"` // ГГГГ-ММ-ДД
	CreatedAt   time.Time `json:"created_at"`
}

// FirewoodStock: остатки дров, м³.
type FirewoodStock struct {
	RawM3 float64 `json:"raw_m3"` // сырьё (дрова, не переработанные)
}

// StockAdjustment: запись журнала корректировок склада (отрицательные значения = списание).
type StockAdjustment struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"` // "board" или "log"
	ItemID      int64     `json:"item_id"`
	ItemLabel   string    `json:"item_label"`
	DeltaCount  int       `json:"delta_count"`
	DeltaVolume float64   `json:"delta_volume"`
	Reason      string    `json:"reason"`
	Note        string    `json:"note"`
	CreatedBy   int64     `json:"created_by"`
	UserName    string    `json:"user_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrderPayments: заказ с итогами и историей оплат (для окна «Оплата»).
type OrderPayments struct {
	OrderID      int64       `json:"order_id"`
	ContractorID int64       `json:"contractor_id"`
	Status       OrderStatus `json:"status"`
	TotalPrice   float64     `json:"total_price"`
	PaidAmount   float64     `json:"paid_amount"`
	Payments     []Payment   `json:"payments"`
}

// DebtOrder: завершённый заказ, по которому клиент ещё не расплатился.
type DebtOrder struct {
	OrderID      int64     `json:"order_id"`
	ContractorID int64     `json:"contractor_id"`
	CreatedAt    time.Time `json:"created_at"`
	Total        float64   `json:"total"`
	Paid         float64   `json:"paid"`
	Due          float64   `json:"due"`
}

type StatisticsSummary struct {
	TotalLogsArrivedVolume float64 `json:"total_logs_arrived_volume"`
	TotalLogsSawedVolume   float64 `json:"total_logs_sawed_volume"`
	TotalSalesVolume       float64 `json:"total_sales_volume"`
	TotalSalesAmount       float64 `json:"total_sales_amount"`
}

type BoardFilter struct {
	Species  string
	Grade    string
	HeightMM int
	WidthMM  int
	LengthMM int
}

type LogFilter struct {
	Species    string
	LengthMM   int
	DiameterMM int
}

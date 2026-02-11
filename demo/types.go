package demo

// OrderEvent 订单事件
type OrderEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	Timestamp  int64   `json:"timestamp"`
}

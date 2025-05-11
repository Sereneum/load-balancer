package storage

type BucketConfig struct {
	Capacity int64   `json:"capacity"`     // Макс. токенов
	Rate     float64 `json:"rate_per_sec"` // Токенов в секунду
}

type Interface interface {
	GetBucket(clientID string) (*BucketConfig, error)
	SetBucket(clientID string, cfg BucketConfig) error
	TakeToken(clientID string) (bool, error)
	RefillTokens(clientID string) error
	Close() error
}

package dto

type ScrollResult[T any] struct {
	Data    []T   `json:"list"`
	MinTime int64 `json:"minTime"`
	Offset  int   `json:"offset"`
}

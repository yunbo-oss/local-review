package dto

type Result[T any] struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	Data     T      `json:"data"`
	Total    int64  `json:"total"`
}

func Ok[T any]() Result[T] {
	var zeroValue T
	return Result[T]{
		Success:  true,
		ErrorMsg: "",
		Data:     zeroValue,
		Total:    0,
	}
}

func OkWithData[T any](data T) Result[T] {
	return Result[T]{
		Success:  true,
		ErrorMsg: "",
		Data:     data,
		Total:    0,
	}
}

func OkWithList[T any](data []T, total int64) Result[[]T] {
	return Result[[]T]{
		Success:  true,
		ErrorMsg: "",
		Data:     data,
		Total:    total,
	}
}

func Fail[T any](errorMsg string) Result[T] {
	var zeroValue T
	return Result[T]{
		Success:  false,
		ErrorMsg: errorMsg,
		Data:     zeroValue,
		Total:    0,
	}
}

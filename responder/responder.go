package responder

type ResponseStructure struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

func New(data interface{}) ResponseStructure {
	return ResponseStructure{
		Success: true,
		Data:    data,
	}
}

func Error(data interface{}) ResponseStructure {
	return ResponseStructure{
		Success: false,
		Data:    data,
	}
}

package http

import "github.com/go-playground/validator/v10"

// Глобальный экземпляр валидатора
var Validate = validator.New()

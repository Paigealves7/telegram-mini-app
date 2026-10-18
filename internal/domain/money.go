package domain

import "math"

// Модель цены: Price у позиции = цена за 1 м³ (руб).
// Сумма позиции = цена за м³ × объём позиции.

// Round2 округляет сумму до копеек.
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// LineTotal — сумма по позиции: цена за м³ × объём.
func LineTotal(pricePerM3, volumeM3 float64) float64 {
	return Round2(pricePerM3 * volumeM3)
}

// ItemVolume — объём count штук позиции склада пропорционально её остатку.
// Для всего остатка возвращается точный объём позиции, без накопления ошибок округления.
func ItemVolume(boardCount int, boardVolume float64, count int) float64 {
	if boardCount <= 0 {
		return 0
	}
	if count == boardCount {
		return boardVolume
	}
	return math.Round(boardVolume/float64(boardCount)*float64(count)*1000) / 1000
}

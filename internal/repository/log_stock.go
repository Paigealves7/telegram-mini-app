package repository

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrLogNotFound      = errors.New("партия сырья не найдена")
	ErrInsufficientLogs = errors.New("недостаточно сырья в партии")
)

const volumeTolerance = 0.0005 // допуск на округление объёма до 3 знаков

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// consumeLog считает остаток партии бревен после распила used м³.
// Число бревен пересчитывается пропорционально объёму; пока объём остаётся, в партии не меньше одного бревна.
func consumeLog(remVol float64, remCount int, used float64) (newVol float64, newCount int, err error) {
	if used > remVol+volumeTolerance {
		left := strings.Replace(fmt.Sprintf("%.3f", remVol), ".", ",", 1)
		return 0, 0, fmt.Errorf("%w: осталось %s м³", ErrInsufficientLogs, left)
	}
	newVol = round3(remVol - used)
	if newVol < volumeTolerance {
		return 0, 0, nil
	}
	newCount = int(math.Round(float64(remCount) * newVol / remVol))
	if newCount < 1 {
		newCount = 1
	}
	if newCount > remCount {
		newCount = remCount
	}
	return newVol, newCount, nil
}

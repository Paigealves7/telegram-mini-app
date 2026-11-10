package service

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"telegram-mini-app/internal/domain"
)

// groupThousands: "1234567" -> "1 234 567".
func groupThousands(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < n; i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// formatMoney: 1234567.891 -> "1 234 567,89".
func formatMoney(v float64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	intPart, frac, _ := strings.Cut(strconv.FormatFloat(v, 'f', 2, 64), ".")
	return sign + groupThousands(intPart) + "," + frac
}

// formatVolume: 3.421 -> "3,421".
func formatVolume(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 3, 64), ".", ",", 1)
}

func formatInt(n int64) string {
	if n < 0 {
		return "-" + groupThousands(strconv.FormatInt(-n, 10))
	}
	return groupThousands(strconv.FormatInt(n, 10))
}

// itemTitle: "Сосна 50×150×6000 мм, сорт 1".
func itemTitle(l domain.OrderLine) string {
	species := strings.TrimSpace(l.Species)
	if l.HeightMM == 0 && l.WidthMM == 0 && l.LengthMM == 0 && species == "" {
		return fmt.Sprintf("Позиция склада №%d", l.BoardID)
	}
	t := fmt.Sprintf("%d×%d×%d мм", l.HeightMM, l.WidthMM, l.LengthMM)
	if species != "" {
		t = species + " " + t
	}
	if g := strings.TrimSpace(l.Grade); g != "" {
		t += ", сорт " + g
	}
	return t
}

var statusLabels = map[domain.OrderStatus]string{
	domain.OrderStatusNew:        "Новый",
	domain.OrderStatusInProgress: "В работе",
	domain.OrderStatusCompleted:  "Завершён",
	domain.OrderStatusCanceled:   "Отменён",
}

func statusLabel(s domain.OrderStatus) string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	return string(s)
}

// ---------- сумма прописью ----------

var (
	onesM    = []string{"", "один", "два", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять"}
	onesF    = []string{"", "одна", "две", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять"}
	teens    = []string{"десять", "одиннадцать", "двенадцать", "тринадцать", "четырнадцать", "пятнадцать", "шестнадцать", "семнадцать", "восемнадцать", "девятнадцать"}
	tensW    = []string{"", "", "двадцать", "тридцать", "сорок", "пятьдесят", "шестьдесят", "семьдесят", "восемьдесят", "девяносто"}
	hundreds = []string{"", "сто", "двести", "триста", "четыреста", "пятьсот", "шестьсот", "семьсот", "восемьсот", "девятьсот"}
)

// plural выбирает форму слова: 1 рубль, 2 рубля, 5 рублей.
func plural(n int64, one, few, many string) string {
	n = n % 100
	if n >= 11 && n <= 19 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}

// triadWords — число 1..999 словами.
func triadWords(n int64, feminine bool) string {
	var parts []string
	h, t, o := n/100, (n/10)%10, n%10
	if h > 0 {
		parts = append(parts, hundreds[h])
	}
	if t == 1 {
		parts = append(parts, teens[o])
	} else {
		if t > 1 {
			parts = append(parts, tensW[t])
		}
		if o > 0 {
			if feminine {
				parts = append(parts, onesF[o])
			} else {
				parts = append(parts, onesM[o])
			}
		}
	}
	return strings.Join(parts, " ")
}

type scaleUnit struct {
	one, few, many string
	feminine       bool
}

var scaleUnits = []scaleUnit{
	{},
	{"тысяча", "тысячи", "тысяч", true},
	{"миллион", "миллиона", "миллионов", false},
	{"миллиард", "миллиарда", "миллиардов", false},
}

func numberWords(n int64) string {
	if n == 0 {
		return "ноль"
	}
	var parts []string
	for i := 0; n > 0 && i < len(scaleUnits); i++ {
		g := n % 1000
		n /= 1000
		if g == 0 {
			continue
		}
		w := triadWords(g, scaleUnits[i].feminine)
		if i > 0 {
			w += " " + plural(g, scaleUnits[i].one, scaleUnits[i].few, scaleUnits[i].many)
		}
		parts = append([]string{w}, parts...)
	}
	return strings.Join(parts, " ")
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// amountInWords: 1234.56 -> "Одна тысяча двести тридцать четыре рубля 56 копеек".
func amountInWords(v float64) string {
	total := int64(math.Round(v * 100))
	if total < 0 {
		total = 0
	}
	rub, kop := total/100, total%100
	if rub >= 1000000000000 {
		return formatMoney(v) + " руб."
	}
	w := numberWords(rub) + " " + plural(rub, "рубль", "рубля", "рублей") +
		fmt.Sprintf(" %02d ", kop) + plural(kop, "копейка", "копейки", "копеек")
	return upperFirst(w)
}
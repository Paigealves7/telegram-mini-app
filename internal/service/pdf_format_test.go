package service

import (
	"testing"

	"telegram-mini-app/internal/domain"
)

func TestAmountInWords(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "Ноль рублей 00 копеек"},
		{1, "Один рубль 00 копеек"},
		{2, "Два рубля 00 копеек"},
		{5, "Пять рублей 00 копеек"},
		{11, "Одиннадцать рублей 00 копеек"},
		{21.5, "Двадцать один рубль 50 копеек"},
		{105.02, "Сто пять рублей 02 копейки"},
		{1234.56, "Одна тысяча двести тридцать четыре рубля 56 копеек"},
		{2000, "Две тысячи рублей 00 копеек"},
		{11000.01, "Одиннадцать тысяч рублей 01 копейка"},
		{21000, "Двадцать одна тысяча рублей 00 копеек"},
		{121000, "Сто двадцать одна тысяча рублей 00 копеек"},
		{362000, "Триста шестьдесят две тысячи рублей 00 копеек"},
		{1000000, "Один миллион рублей 00 копеек"},
		{2500000.99, "Два миллиона пятьсот тысяч рублей 99 копеек"},
		{1000000000, "Один миллиард рублей 00 копеек"},
	}
	for _, tt := range tests {
		if got := amountInWords(tt.in); got != tt.want {
			t.Errorf("amountInWords(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0.0, "0,00"},
		{9000.0, "9 000,00"},
		{1234567.891, "1 234 567,89"},
		{12.5, "12,50"},
		{100.0, "100,00"},
		{999999.99, "999 999,99"},
	}
	for _, tt := range tests {
		if got := formatMoney(tt.in); got != tt.want {
			t.Errorf("formatMoney(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatVolumeAndInt(t *testing.T) {
	if got := formatVolume(3.421); got != "3,421" {
		t.Errorf("formatVolume = %q", got)
	}
	if got := formatInt(1234567); got != "1 234 567" {
		t.Errorf("formatInt = %q", got)
	}
}

func TestItemTitle(t *testing.T) {
	l := domain.OrderLine{BoardID: 7, Species: "Сосна", Grade: "1", HeightMM: 50, WidthMM: 150, LengthMM: 6000}
	if got := itemTitle(l); got != "Сосна 50×150×6000 мм, сорт 1" {
		t.Errorf("itemTitle = %q", got)
	}
	if got := itemTitle(domain.OrderLine{BoardID: 7}); got != "Позиция склада №7" {
		t.Errorf("itemTitle(empty) = %q", got)
	}
}

package service

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"telegram-mini-app/internal/domain"
)

// Проверяет, что накладная реально собирается (включая перенос на вторую страницу).
// Нужны шрифты DejaVu: в образе они ставятся через apk, локально задайте PDF_FONT_DIR.
func TestRenderPDF(t *testing.T) {
	dir := findFontDir()
	if dir == "" {
		t.Skip("шрифты DejaVu не найдены (задайте PDF_FONT_DIR)")
	}
	s := &PDFService{company: "Лесопилка Тест", info: "г. Пример, ул. Лесная, 1 · тел. +7 000 000-00-00"}
	s.initFonts(dir)
	if s.active == nil {
		t.Fatalf("ни один шрифт не подошёл: %v", s.fontErr)
	}
	t.Logf("шрифт: %s", s.active.name)

	for _, n := range []int{0, 3, 60} {
		order := &domain.Order{ID: 5, ContractorID: 1, Status: domain.OrderStatusNew, CreatedAt: time.Now(),
			DeliveryPrice: 1500, Notes: "Позвонить за час до приезда"}
		var lines []domain.OrderLine
		for i := 0; i < n; i++ {
			lines = append(lines, domain.OrderLine{BoardID: int64(i + 1), Species: "Лиственница", Grade: "1",
				HeightMM: 50, WidthMM: 150, LengthMM: 6000, Count: 40, VolumeM3: 1.8, Price: 9000})
		}
		data, err := s.render(order, lines, "ООО «Ромашка»", "Иванов И.")
		if err != nil {
			t.Fatalf("%d позиций: %v", n, err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 2000 {
			t.Fatalf("%d позиций: странный PDF (%d байт)", n, len(data))
		}
		t.Log(fmt.Sprintf("%d позиций -> %d байт", n, len(data)))
	}
}

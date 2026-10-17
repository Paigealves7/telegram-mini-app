package service

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"telegram-mini-app/internal/domain"

	"github.com/go-pdf/fpdf"
)

type PDFService struct{}

func NewPDFService() *PDFService {
	return &PDFService{}
}

// Автоматически скачиваем кириллический шрифт, если его нет на сервере
func (s *PDFService) ensureCyrillicFont() error {
	fontPath := "Roboto-Regular.ttf"
	if _, err := os.Stat(fontPath); os.IsNotExist(err) {
		resp, err := http.Get("https://github.com/googlefonts/roboto/raw/main/src/hinted/Roboto-Regular.ttf")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		out, err := os.Create(fontPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	}
	return nil
}

func (s *PDFService) GenerateOrderPDF(w io.Writer, order *domain.Order, items []domain.OrderItem) error {
	if err := s.ensureCyrillicFont(); err != nil {
		return fmt.Errorf("ошибка загрузки шрифта: %v", err)
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8Font("Roboto", "", "Roboto-Regular.ttf")
	pdf.AddPage()

	// Заголовок
	pdf.SetFont("Roboto", "", 16)
	pdf.CellFormat(190, 10, fmt.Sprintf("Накладная к Заказу #%d", order.ID), "0", 1, "C", false, 0, "")
	pdf.Ln(10)

	// Информация о заказе
	pdf.SetFont("Roboto", "", 12)
	pdf.CellFormat(190, 8, fmt.Sprintf("ID Клиента: %d", order.ContractorID), "0", 1, "L", false, 0, "")
	pdf.CellFormat(190, 8, fmt.Sprintf("Статус: %s", order.Status), "0", 1, "L", false, 0, "")
	pdf.Ln(5)

	// Шапка таблицы
	pdf.SetFont("Roboto", "", 10)
	pdf.CellFormat(30, 8, "ID Доски", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Кол-во (шт)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Объем (м3)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Цена (руб)", "1", 0, "C", false, 0, "")
	pdf.CellFormat(40, 8, "Сумма (руб)", "1", 1, "C", false, 0, "")

	// Строки таблицы
	var totalSum float64
	for _, item := range items {
		sum := item.Price * float64(item.Count)
		totalSum += sum

		pdf.CellFormat(30, 8, fmt.Sprintf("%d", item.BoardID), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%d", item.Count), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%.3f", item.VolumeM3), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%.2f", item.Price), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, fmt.Sprintf("%.2f", sum), "1", 1, "C", false, 0, "")
	}

	pdf.Ln(5)

	// Итого
	pdf.SetFont("Roboto", "", 12)
	pdf.CellFormat(190, 8, fmt.Sprintf("Доставка: %.2f руб. | Наценка: %.2f руб.", order.DeliveryPrice, order.ExtraPrice), "0", 1, "R", false, 0, "")
	totalSum += order.DeliveryPrice + order.ExtraPrice
	pdf.CellFormat(190, 8, fmt.Sprintf("ИТОГО К ОПЛАТЕ: %.2f руб.", totalSum), "0", 1, "R", false, 0, "")

	// Пишем готовый PDF прямо в HTTP-ответ
	return pdf.Output(w)
}

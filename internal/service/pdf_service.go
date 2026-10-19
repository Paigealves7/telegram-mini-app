package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"telegram-mini-app/internal/domain"
	"telegram-mini-app/internal/repository"

	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
)

var ErrPDFFonts = errors.New("шрифты для PDF не найдены на сервере")

// OrderPDF — готовый документ.
type OrderPDF struct {
	Data     []byte
	Filename string
}

func PDFFileName(orderID int64) string      { return fmt.Sprintf("Накладная_%d.pdf", orderID) }
func PDFFileNameASCII(orderID int64) string { return fmt.Sprintf("nakladnaya_%d.pdf", orderID) }

type PDFService struct {
	repo    *repository.Repository
	linkKey []byte
	fontDir string
	company string
	info    string
}

// NewPDFService: шрифты ищутся на диске один раз при старте (в образе их ставит apk add font-dejavu),
// во время работы ничего из интернета не скачивается.
func NewPDFService(repo *repository.Repository, secret string) *PDFService {
	key := sha256.Sum256([]byte("pdf-link:" + secret))
	s := &PDFService{
		repo:    repo,
		linkKey: key[:],
		company: envOr("COMPANY_NAME", "Лесопилка"),
		info:    strings.TrimSpace(os.Getenv("COMPANY_INFO")),
		fontDir: findFontDir(),
	}
	if s.fontDir == "" {
		log.Printf("PDF: ВНИМАНИЕ: не найдены DejaVuSans.ttf и DejaVuSans-Bold.ttf, накладные работать не будут (установите font-dejavu или задайте PDF_FONT_DIR)")
	} else {
		log.Printf("PDF: шрифты найдены в %s", s.fontDir)
	}
	return s
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func findFontDir() string {
	dirs := []string{os.Getenv("PDF_FONT_DIR"), "/usr/share/fonts/dejavu", "/usr/share/fonts/truetype/dejavu", "/usr/share/fonts/TTF", "fonts"}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if fileExists(filepath.Join(d, "DejaVuSans.ttf")) && fileExists(filepath.Join(d, "DejaVuSans-Bold.ttf")) {
			return d
		}
	}
	return ""
}

// BuildOrderPDF собирает накладную целиком в памяти. Клиент получает файл только если всё удалось.
func (s *PDFService) BuildOrderPDF(ctx context.Context, orderID int64) (*OrderPDF, error) {
	if s.fontDir == "" {
		return nil, ErrPDFFonts
	}

	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrOrderNotFound
		}
		return nil, fmt.Errorf("заказ: %w", err)
	}
	lines, err := s.repo.GetOrderLines(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("позиции заказа: %w", err)
	}

	client, err := s.repo.GetContractorName(ctx, order.ContractorID)
	if err != nil || strings.TrimSpace(client) == "" {
		client = fmt.Sprintf("Клиент №%d", order.ContractorID)
	}
	author := ""
	if u, err := s.repo.GetUserByID(ctx, order.MasterID); err == nil {
		if strings.HasPrefix(u.Username, "tg_") {
			author = fmt.Sprintf("Сотрудник №%d", u.ID)
		} else {
			author = u.Username
		}
	}

	data, err := s.render(order, lines, client, author)
	if err != nil {
		return nil, fmt.Errorf("генерация PDF: %w", err)
	}
	return &OrderPDF{Data: data, Filename: PDFFileName(orderID)}, nil
}

const (
	pageW    = 210.0
	pageH    = 297.0
	marginX  = 15.0
	contentW = pageW - 2*marginX
	bottomY  = pageH - 28 // ниже этой отметки строки таблицы не пишем
	fontName = "DejaVu"
)

type tableCol struct {
	w     float64
	title string
	align string
}

var tableCols = []tableCol{
	{9, "№", "C"},
	{66, "Наименование", "L"},
	{20, "Кол-во, шт", "R"},
	{25, "Объём, м³", "R"},
	{30, "Цена за м³, руб.", "R"},
	{30, "Сумма, руб.", "R"},
}

// fitText подбирает размер шрифта, чтобы текст влез в maxW; если и 6.5 pt мало, обрезает с «…».
// Шрифт остаётся установленным на выбранный размер.
func fitText(pdf *fpdf.Fpdf, style, text string, maxW, size float64) string {
	for sz := size; sz >= 6.5; sz -= 0.5 {
		pdf.SetFont(fontName, style, sz)
		if pdf.GetStringWidth(text) <= maxW {
			return text
		}
	}
	pdf.SetFont(fontName, style, 6.5)
	r := []rune(text)
	for len(r) > 1 && pdf.GetStringWidth(string(r)+"…") > maxW {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (s *PDFService) render(order *domain.Order, lines []domain.OrderLine, client, author string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8Font(fontName, "", filepath.Join(s.fontDir, "DejaVuSans.ttf"))
	pdf.AddUTF8Font(fontName, "B", filepath.Join(s.fontDir, "DejaVuSans-Bold.ttf"))
	pdf.SetMargins(marginX, 15, marginX)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetTitle(fmt.Sprintf("Накладная № %d", order.ID), true)
	pdf.SetAuthor(s.company, true)
	pdf.SetCreator("Лесопилка", true)
	pdf.AliasNbPages("")

	footerLeft := fmt.Sprintf("Накладная № %d · %s", order.ID, s.company)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetDrawColor(215, 220, 215)
		pdf.SetLineWidth(0.2)
		pdf.Line(marginX, pdf.GetY(), pageW-marginX, pdf.GetY())
		pdf.SetFont(fontName, "", 8)
		pdf.SetTextColor(130, 130, 130)
		pdf.CellFormat(120, 8, footerLeft, "", 0, "L", false, 0, "")
		pdf.CellFormat(60, 8, fmt.Sprintf("Стр. %d из {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()

	// ----- шапка -----
	pdf.SetTextColor(46, 92, 62)
	name := fitText(pdf, "B", s.company, 98, 16)
	pdf.CellFormat(100, 8, name, "", 0, "L", false, 0, "")
	pdf.SetTextColor(30, 30, 30)
	pdf.SetFont(fontName, "B", 16)
	pdf.CellFormat(80, 8, fmt.Sprintf("НАКЛАДНАЯ № %d", order.ID), "", 1, "R", false, 0, "")

	y := pdf.GetY()
	pdf.SetFont(fontName, "", 8)
	pdf.SetTextColor(120, 120, 120)
	if s.info != "" {
		pdf.MultiCell(100, 4, s.info, "", "L", false)
	}
	infoEnd := pdf.GetY()
	if !order.CreatedAt.IsZero() {
		pdf.SetXY(marginX+100, y)
		pdf.SetFont(fontName, "", 10)
		pdf.SetTextColor(30, 30, 30)
		pdf.CellFormat(80, 5, "от "+order.CreatedAt.In(time.Local).Format("02.01.2006"), "", 0, "R", false, 0, "")
	}
	if infoEnd < y+5 {
		infoEnd = y + 5
	}
	y = infoEnd + 3
	pdf.SetDrawColor(46, 92, 62)
	pdf.SetLineWidth(0.6)
	pdf.Line(marginX, y, pageW-marginX, y)
	pdf.SetXY(marginX, y+5)

	// ----- реквизиты -----
	kv := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		pdf.SetX(marginX)
		pdf.SetFont(fontName, "", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(32, 5.5, label, "", 0, "L", false, 0, "")
		pdf.SetFont(fontName, "B", 10)
		pdf.SetTextColor(30, 30, 30)
		pdf.MultiCell(contentW-32, 5.5, value, "", "L", false)
	}
	kv("Клиент:", client)
	kv("Оформил:", author)
	kv("Статус:", statusLabel(order.Status))
	kv("Примечание:", order.Notes)
	pdf.Ln(4)

	// ----- таблица -----
	drawHeader := func() {
		pdf.SetFillColor(46, 92, 62)
		pdf.SetTextColor(255, 255, 255)
		for i, c := range tableCols {
			txt := fitText(pdf, "B", c.title, c.w-2, 8.5)
			ln := 0
			if i == len(tableCols)-1 {
				ln = 1
			}
			pdf.CellFormat(c.w, 9, txt, "", ln, "C", true, 0, "")
		}
	}
	drawHeader()

	const rowH = 8.0
	var totalVol, totalSum float64
	if len(lines) == 0 {
		pdf.SetFont(fontName, "", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(contentW, 10, "Позиции отсутствуют", "B", 1, "C", false, 0, "")
	}
	for i, l := range lines {
		if pdf.GetY()+rowH > bottomY {
			pdf.AddPage()
			drawHeader()
		}
		sum := domain.LineTotal(l.Price, l.VolumeM3)
		totalVol += l.VolumeM3
		totalSum += sum

		fill := i%2 == 1
		pdf.SetFillColor(244, 247, 244)
		pdf.SetDrawColor(215, 220, 215)
		pdf.SetLineWidth(0.2)
		pdf.SetTextColor(30, 30, 30)
		cells := []string{
			strconv.Itoa(i + 1), itemTitle(l), formatInt(l.Count),
			formatVolume(l.VolumeM3), formatMoney(l.Price), formatMoney(sum),
		}
		for j, c := range tableCols {
			txt := fitText(pdf, "", cells[j], c.w-2, 9)
			ln := 0
			if j == len(tableCols)-1 {
				ln = 1
			}
			align := c.align
			if j == 0 {
				align = "C"
			}
			pdf.CellFormat(c.w, rowH, txt, "B", ln, align, fill, 0, "")
		}
	}

	// ----- итоги -----
	grand := domain.Round2(totalSum + order.DeliveryPrice + order.ExtraPrice)
	if pdf.GetY()+95 > bottomY {
		pdf.AddPage()
	}
	pdf.Ln(4)

	const labelW, valW = 60.0, 40.0
	totalRow := func(label, value string, accent bool) {
		pdf.SetX(pageW - marginX - labelW - valW)
		if accent {
			pdf.SetFillColor(46, 92, 62)
			pdf.SetTextColor(255, 255, 255)
			pdf.SetFont(fontName, "B", 11)
		} else {
			pdf.SetTextColor(30, 30, 30)
			pdf.SetFont(fontName, "", 10)
		}
		pdf.CellFormat(labelW, 7, label, "", 0, "R", accent, 0, "")
		pdf.CellFormat(valW, 7, value, "", 1, "R", accent, 0, "")
	}
	totalRow("Итого объём:", formatVolume(totalVol)+" м³", false)
	totalRow("Сумма по позициям:", formatMoney(totalSum)+" руб.", false)
	if order.DeliveryPrice > 0 {
		totalRow("Доставка:", formatMoney(order.DeliveryPrice)+" руб.", false)
	}
	if order.ExtraPrice > 0 {
		totalRow("Дополнительно:", formatMoney(order.ExtraPrice)+" руб.", false)
	}
	pdf.Ln(1)
	totalRow("К ОПЛАТЕ:", formatMoney(grand)+" руб.", true)

	pdf.Ln(4)
	pdf.SetX(marginX)
	pdf.SetFont(fontName, "", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(32, 5, "Сумма прописью:", "", 0, "L", false, 0, "")
	pdf.SetFont(fontName, "B", 9.5)
	pdf.SetTextColor(30, 30, 30)
	pdf.MultiCell(contentW-32, 5, amountInWords(grand)+".", "", "L", false)

	// ----- подписи -----
	if pdf.GetY()+30 > bottomY {
		pdf.AddPage()
	}
	ys := pdf.GetY() + 18
	pdf.SetDrawColor(120, 120, 120)
	pdf.SetLineWidth(0.3)
	pdf.Line(marginX, ys, marginX+75, ys)
	pdf.Line(pageW-marginX-75, ys, pageW-marginX, ys)
	pdf.SetFont(fontName, "", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(marginX, ys+1)
	pdf.CellFormat(75, 4, "Отпустил: подпись / расшифровка", "", 0, "L", false, 0, "")
	pdf.SetXY(pageW-marginX-75, ys+1)
	pdf.CellFormat(75, 4, "Получил: подпись / расшифровка", "", 0, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

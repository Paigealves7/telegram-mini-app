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

// PDFError — ошибка сборки накладной с названием этапа: по нему сразу видно, что именно сломалось.
type PDFError struct {
	Stage string
	Err   error
}

func (e *PDFError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *PDFError) Unwrap() error { return e.Err }

// fontPair — пара шрифтов (обычный + жирный), прочитанных в память один раз при старте.
type fontPair struct {
	name    string
	regular []byte
	bold    []byte
}

type PDFService struct {
	repo    *repository.Repository
	linkKey []byte
	company string
	info    string
	active  *fontPair // шрифт, прошедший самопроверку при старте
	fontErr error     // почему ни один шрифт не подошёл
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
	}
	s.initFonts(findFontDir())
	return s
}

func sampleOrder() *domain.Order {
	return &domain.Order{ID: 1, ContractorID: 1, Status: domain.OrderStatusNew, CreatedAt: time.Now(), DeliveryPrice: 100, Notes: "проверка"}
}

func sampleLines() []domain.OrderLine {
	return []domain.OrderLine{{BoardID: 1, Species: "Сосна", Grade: "1", HeightMM: 50, WidthMM: 150, LengthMM: 6000, Count: 10, VolumeM3: 0.45, Price: 9000}}
}

// initFonts читает шрифты в память и ПРОБНО собирает накладную каждым вариантом.
// Берётся первый, который реально работает с библиотекой; причина отказа попадает в лог.
// Сначала DejaVuSansCondensed: именно им проверяются примеры самой библиотеки fpdf.
func (s *PDFService) initFonts(dir string) {
	s.active, s.fontErr = nil, nil
	if dir == "" {
		s.fontErr = ErrPDFFonts
		log.Printf("PDF: ВНИМАНИЕ: не найдены шрифты DejaVu (установите font-dejavu или задайте PDF_FONT_DIR): накладные работать не будут")
		return
	}
	var lastErr error
	for _, base := range []string{"DejaVuSansCondensed", "DejaVuSans"} {
		reg, errR := os.ReadFile(filepath.Join(dir, base+".ttf"))
		bold, errB := os.ReadFile(filepath.Join(dir, base+"-Bold.ttf"))
		if errR != nil || errB != nil {
			continue
		}
		fp := &fontPair{name: base, regular: reg, bold: bold}
		if _, err := s.renderWith(fp, sampleOrder(), sampleLines(), "Проверка", "Проверка"); err != nil {
			lastErr = fmt.Errorf("шрифт %s: %w", base, err)
			log.Printf("PDF: шрифт %s не подошёл: %v", base, err)
			continue
		}
		s.active = fp
		log.Printf("PDF: самопроверка пройдена, шрифт %s (%s)", base, dir)
		return
	}
	if lastErr == nil {
		lastErr = ErrPDFFonts
	}
	s.fontErr = lastErr
	log.Printf("PDF: ВНИМАНИЕ: накладные работать не будут: %v", lastErr)
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
		if (fileExists(filepath.Join(d, "DejaVuSans.ttf")) && fileExists(filepath.Join(d, "DejaVuSans-Bold.ttf"))) ||
			(fileExists(filepath.Join(d, "DejaVuSansCondensed.ttf")) && fileExists(filepath.Join(d, "DejaVuSansCondensed-Bold.ttf"))) {
			return d
		}
	}
	return ""
}

// BuildOrderPDF собирает накладную целиком в памяти. Клиент получает файл только если всё удалось.
func (s *PDFService) BuildOrderPDF(ctx context.Context, orderID int64) (*OrderPDF, error) {
	if s.active == nil {
		if s.fontErr != nil && !errors.Is(s.fontErr, ErrPDFFonts) {
			return nil, &PDFError{Stage: "шрифты", Err: s.fontErr}
		}
		return nil, ErrPDFFonts
	}

	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrOrderNotFound
		}
		return nil, &PDFError{Stage: "заказ", Err: err}
	}
	lines, err := s.repo.GetOrderLines(ctx, orderID)
	if err != nil {
		return nil, &PDFError{Stage: "позиции заказа", Err: err}
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
		return nil, &PDFError{Stage: "вёрстка", Err: err}
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
	return s.renderWith(s.active, order, lines, client, author)
}

func (s *PDFService) renderWith(fp *fontPair, order *domain.Order, lines []domain.OrderLine, client, author string) (data []byte, err error) {
	// Библиотека может паниковать на «неудобных» шрифтах: превращаем панику в обычную ошибку.
	defer func() {
		if rec := recover(); rec != nil {
			data, err = nil, fmt.Errorf("паника библиотеки PDF: %v", rec)
		}
	}()

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AliasNbPages("") // до добавления шрифтов: так задумано в библиотеке для UTF-8 шрифтов
	pdf.AddUTF8FontFromBytes(fontName, "", fp.regular)
	pdf.AddUTF8FontFromBytes(fontName, "B", fp.bold)
	if e := pdf.Error(); e != nil {
		return nil, fmt.Errorf("загрузка шрифта: %w", e)
	}
	pdf.SetMargins(marginX, 15, marginX)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetTitle(fmt.Sprintf("Накладная № %d", order.ID), true)
	pdf.SetAuthor(s.company, true)
	pdf.SetCreator("Лесопилка", true)

	footerLeft := fmt.Sprintf("Накладная № %d · %s", order.ID, s.company)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetDrawColor(205, 205, 205)
		pdf.SetLineWidth(0.2)
		pdf.Line(marginX, pdf.GetY(), pageW-marginX, pdf.GetY())
		pdf.SetFont(fontName, "", 8)
		pdf.SetTextColor(130, 130, 130)
		pdf.CellFormat(120, 8, footerLeft, "", 0, "L", false, 0, "")
		pdf.CellFormat(60, 8, fmt.Sprintf("Стр. %d из {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()

	// ----- шапка -----
	pdf.SetTextColor(30, 30, 30)
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
	pdf.SetDrawColor(60, 60, 60)
	pdf.SetLineWidth(0.4)
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
		// Шапка таблицы без заливки: тонкие линии сверху и снизу (так накладная экономно печатается)
		pdf.SetTextColor(30, 30, 30)
		pdf.SetDrawColor(60, 60, 60)
		pdf.SetLineWidth(0.3)
		for i, c := range tableCols {
			txt := fitText(pdf, "B", c.title, c.w-2, 8.5)
			ln := 0
			if i == len(tableCols)-1 {
				ln = 1
			}
			pdf.CellFormat(c.w, 9, txt, "TB", ln, "C", false, 0, "")
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

		fill := false
		pdf.SetDrawColor(215, 215, 215)
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
		border := ""
		pdf.SetTextColor(30, 30, 30)
		if accent {
			pdf.SetDrawColor(60, 60, 60)
			pdf.SetLineWidth(0.3)
			pdf.SetFont(fontName, "B", 12)
			border = "T" // итог выделен жирным шрифтом и линией сверху, без заливки
		} else {
			pdf.SetFont(fontName, "", 10)
		}
		pdf.CellFormat(labelW, 8, label, border, 0, "R", false, 0, "")
		pdf.CellFormat(valW, 8, value, border, 1, "R", false, 0, "")
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

	if e := pdf.Error(); e != nil {
		return nil, fmt.Errorf("построение страницы: %w", e)
	}
	var buf bytes.Buffer
	if e := pdf.Output(&buf); e != nil {
		return nil, fmt.Errorf("вывод файла: %w", e)
	}
	return buf.Bytes(), nil
}

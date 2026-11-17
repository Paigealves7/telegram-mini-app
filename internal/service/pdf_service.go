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
		if _, err := s.renderWith(fp, sampleOrder(), sampleLines(), "Проверка", "Проверка", 0); err != nil {
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
	author := "Не указан"
	if order.MasterID > 0 {
		author = fmt.Sprintf("Сотрудник №%d", order.MasterID)
	}
	if u, err := s.repo.GetUserByID(ctx, order.MasterID); err == nil {
		if name := strings.TrimSpace(u.DisplayName); name != "" && name != "." {
			author = name
		} else if username := strings.TrimLeft(strings.TrimSpace(u.TGUsername), "@"); username != "" {
			author = "@" + username
		}
	}

	payments, err := s.repo.GetOrderPayments(ctx, orderID)
	if err != nil {
		return nil, &PDFError{Stage: "оплаты заказа", Err: err}
	}
	data, err := s.renderWith(s.active, order, lines, client, author, payments.PaidAmount)
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
	{8, "№", "C"},
	{62, "Наименование, размер, сорт", "L"},
	{12, "Ед.", "C"},
	{18, "Кол-во, шт", "R"},
	{22, "Объём, м³", "R"},
	{28, "Цена за м³, руб.", "R"},
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

func (s *PDFService) renderWith(fp *fontPair, order *domain.Order, lines []domain.OrderLine, client, author string, paid float64) (data []byte, err error) {
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
		pdf.SetTextColor(130, 130, 130)
		txt := fitText(pdf, "", footerLeft, 118, 8)
		pdf.CellFormat(120, 8, txt, "", 0, "L", false, 0, "")
		pdf.SetFont(fontName, "", 8)
		pdf.CellFormat(60, 8, fmt.Sprintf("Стр. %d из {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()

	// ----- шапка -----
	pdf.SetTextColor(30, 30, 30)
	pdf.SetFont(fontName, "B", 16)
	pdf.CellFormat(contentW, 8, fmt.Sprintf("ТОВАРНАЯ НАКЛАДНАЯ № %d", order.ID), "", 1, "C", false, 0, "")
	date := order.CreatedAt.In(time.Local).Format("02.01.2006")
	pdf.SetFont(fontName, "", 10)
	pdf.CellFormat(contentW, 6, "от "+date, "", 1, "C", false, 0, "")
	y := pdf.GetY() + 3
	pdf.SetDrawColor(60, 60, 60)
	pdf.SetLineWidth(0.4)
	pdf.Line(marginX, y, pageW-marginX, y)
	pdf.SetXY(marginX, y+5)

	// ----- реквизиты -----
	kv := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if pdf.GetY()+5.5 > bottomY {
			pdf.AddPage()
		}
		pdf.SetX(marginX)
		pdf.SetFont(fontName, "", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(32, 5.5, label, "", 0, "L", false, 0, "")
		pdf.SetFont(fontName, "B", 10)
		pdf.SetTextColor(30, 30, 30)
		for _, line := range pdf.SplitText(strings.ReplaceAll(value, "\r", ""), contentW-32) {
			if pdf.GetY()+5.5 > bottomY {
				pdf.AddPage()
			}
			pdf.SetX(marginX + 32)
			pdf.CellFormat(contentW-32, 5.5, line, "", 1, "L", false, 0, "")
		}
	}
	supplier := s.company
	if s.info != "" {
		supplier += "\n" + s.info
	}
	kv("Поставщик:", supplier)
	kv("Покупатель:", client)
	kv("Основание:", fmt.Sprintf("Заказ (заявка) № %d от %s", order.ID, date))
	kv("Ответственный:", author)
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
			pdf.CellFormat(c.w, 9, txt, "1", ln, c.align, false, 0, "")
		}
	}
	if pdf.GetY()+19 > bottomY {
		pdf.AddPage()
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
		pdf.SetFont(fontName, "", 9)
		titleLines := pdf.SplitText(itemTitle(l), tableCols[1].w)
		rowH := max(rowH, 2+4*float64(len(titleLines)))
		if pdf.GetY()+rowH > bottomY {
			pdf.AddPage()
			drawHeader()
		}
		sum := domain.LineTotal(l.Price, l.VolumeM3)
		totalVol += l.VolumeM3
		totalSum += sum

		pdf.SetDrawColor(215, 215, 215)
		pdf.SetLineWidth(0.2)
		pdf.SetTextColor(30, 30, 30)
		unit := "шт / м³"
		if l.FirewoodKind != "" {
			unit = "м³"
		}
		cells := []string{
			strconv.Itoa(i + 1), "", unit, itemCount(l),
			formatVolume(l.VolumeM3), formatMoney(l.Price), formatMoney(sum),
		}
		for j, c := range tableCols {
			if j == 1 {
				x, y := pdf.GetXY()
				pdf.SetFont(fontName, "", 9)
				pdf.CellFormat(c.w, rowH, "", "1", 0, "L", false, 0, "")
				pdf.SetY(y + (rowH-4*float64(len(titleLines)))/2)
				for _, line := range titleLines {
					pdf.SetX(x)
					pdf.CellFormat(c.w, 4, line, "", 1, "L", false, 0, "")
				}
				pdf.SetXY(x+c.w, y)
				continue
			}
			txt := fitText(pdf, "", cells[j], c.w-2, 9)
			ln := 0
			if j == len(tableCols)-1 {
				ln = 1
			}
			pdf.CellFormat(c.w, rowH, txt, "1", ln, c.align, false, 0, "")
		}
	}

	// ----- итоги -----
	grand := domain.Round2(totalSum + order.DeliveryPrice + order.ExtraPrice)
	if pdf.GetY()+110 > bottomY {
		pdf.AddPage()
	}
	pdf.Ln(4)
	totalsY := pdf.GetY()
	pdf.SetFont(fontName, "", 9)
	pdf.MultiCell(76, 5, fmt.Sprintf("Всего отпущено наименований: %d, общим объёмом %s м³ на сумму %s руб.",
		len(lines), formatVolume(totalVol), formatMoney(grand)), "", "L", false)
	summaryEnd := pdf.GetY()
	pdf.SetY(totalsY)

	const labelW, valW = 60.0, 40.0
	totalRow := func(label, value string, accent bool) {
		pdf.SetX(pageW - marginX - labelW - valW)
		border := ""
		style, size := "", 10.0
		pdf.SetTextColor(30, 30, 30)
		if accent {
			pdf.SetDrawColor(60, 60, 60)
			pdf.SetLineWidth(0.3)
			style, size = "B", 12
			border = "T" // итог выделен жирным шрифтом и линией сверху, без заливки
		}
		pdf.SetFont(fontName, style, size)
		pdf.CellFormat(labelW, 8, label, border, 0, "R", false, 0, "")
		txt := fitText(pdf, style, value, valW-2, size)
		pdf.CellFormat(valW, 8, txt, border, 1, "R", false, 0, "")
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
	totalRow("ИТОГО К ОПЛАТЕ:", formatMoney(grand)+" руб.", true)
	if paid > 0 {
		totalRow("Оплачено:", formatMoney(paid)+" руб.", false)
		totalRow("Остаток долга:", formatMoney(max(0, domain.Round2(grand-paid)))+" руб.", false)
	}
	pdf.SetY(max(pdf.GetY(), summaryEnd))

	pdf.Ln(4)
	pdf.SetX(marginX)
	pdf.SetFont(fontName, "", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(32, 5, "Сумма прописью:", "", 0, "L", false, 0, "")
	pdf.SetFont(fontName, "B", 9.5)
	pdf.SetTextColor(30, 30, 30)
	pdf.MultiCell(contentW-32, 5, amountInWords(grand)+".", "", "L", false)

	// ----- подписи -----
	pdf.SetFont(fontName, "", 9)
	sender := "Отпустил (Сдал): _______________ / " + strings.Join(strings.Fields(author), " ") + " /"
	signatureHeight := 10 + 5*float64(max(3, len(pdf.SplitText(sender, 87))))
	if pdf.GetY()+signatureHeight > bottomY {
		pdf.AddPage()
	}
	ys := pdf.GetY() + 5
	pdf.SetFont(fontName, "", 9)
	pdf.SetXY(marginX, ys)
	pdf.MultiCell(87, 5, sender, "", "L", false)
	pdf.SetX(marginX)
	pdf.CellFormat(87, 5, "Должность: ____________________", "", 1, "L", false, 0, "")
	pdf.CellFormat(87, 5, "М.П.", "", 1, "L", false, 0, "")
	pdf.SetXY(marginX+93, ys)
	pdf.MultiCell(87, 5, "Получил (Принял): _______________ / _______________ /\nДолжность: ____________________", "", "L", false)

	if e := pdf.Error(); e != nil {
		return nil, fmt.Errorf("построение страницы: %w", e)
	}
	var buf bytes.Buffer
	if e := pdf.Output(&buf); e != nil {
		return nil, fmt.Errorf("вывод файла: %w", e)
	}
	return buf.Bytes(), nil
}

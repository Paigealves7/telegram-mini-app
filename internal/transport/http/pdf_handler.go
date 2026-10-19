package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"telegram-mini-app/internal/repository"
	"telegram-mini-app/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Срок действия ссылки на скачивание. Клиенты Telegram не умеют передавать заголовок
// Authorization, поэтому файл отдаётся по короткоживущей подписанной ссылке.
const pdfLinkTTL = 10 * time.Minute

func pdfWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseOrderID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

func writePDFError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrOrderNotFound):
		http.Error(w, "заказ не найден", http.StatusNotFound)
	case errors.Is(err, service.ErrPDFFonts):
		http.Error(w, "на сервере не установлены шрифты для PDF", http.StatusServiceUnavailable)
	default:
		log.Printf("PDF: %v", err)
		http.Error(w, "не удалось сформировать PDF", http.StatusInternalServerError)
	}
}

// contentDisposition: ASCII-имя для старых клиентов + UTF-8 имя (RFC 5987) для остальных.
func contentDisposition(kind string, orderID int64) string {
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, kind, service.PDFFileNameASCII(orderID), url.PathEscape(service.PDFFileName(orderID)))
}

// servePDF отдаёт готовый файл целиком: с Content-Length, поддержкой HEAD и Range
// (iOS и просмотрщики PDF докачивают файл частями).
func servePDF(w http.ResponseWriter, r *http.Request, doc *service.OrderPDF, orderID int64, disposition string) {
	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", contentDisposition(disposition, orderID))
	h.Set("Cache-Control", "private, no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, doc.Filename, time.Time{}, bytes.NewReader(doc.Data))
}

// publicBaseURL — адрес, по которому клиент Telegram сможет скачать файл. Лучше задать PUBLIC_URL.
func publicBaseURL(r *http.Request) string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_URL")), "/"); v != "" {
		return v
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if i := strings.Index(host, ","); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}
	scheme := "https"
	if h := strings.Split(host, ":")[0]; h == "localhost" || h == "127.0.0.1" {
		scheme = "http"
	}
	return scheme + "://" + host
}

// GET /api/v1/orders/{id}/pdf — прямое скачивание с токеном в заголовке (для API и веб-входа).
func (h *OrderHandler) DownloadPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(r)
	if !ok {
		http.Error(w, "некорректный ID заказа", http.StatusBadRequest)
		return
	}
	doc, err := h.pdfService.BuildOrderPDF(r.Context(), id)
	if err != nil {
		writePDFError(w, err)
		return
	}
	servePDF(w, r, doc, id, "attachment")
}

type pdfLinkResponse struct {
	DownloadURL string `json:"download_url"`
	ViewURL     string `json:"view_url"`
	FileName    string `json:"file_name"`
	ExpiresIn   int    `json:"expires_in"`
}

// POST /api/v1/orders/{id}/pdf/link — выдаёт короткоживущую ссылку на накладную.
func (h *OrderHandler) PDFLink(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(r)
	if !ok {
		http.Error(w, "некорректный ID заказа", http.StatusBadRequest)
		return
	}
	if _, err := h.repo.GetOrderByID(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "заказ не найден", http.StatusNotFound)
			return
		}
		log.Printf("PDF link: %v", err)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	token := h.pdfService.SignLink(id, pdfLinkTTL)
	link := fmt.Sprintf("%s/files/pdf/%s/%s", publicBaseURL(r), token, service.PDFFileNameASCII(id))
	pdfWriteJSON(w, http.StatusOK, pdfLinkResponse{
		DownloadURL: link,
		ViewURL:     link + "?inline=1",
		FileName:    service.PDFFileName(id),
		ExpiresIn:   int(pdfLinkTTL.Seconds()),
	})
}

// GET|HEAD /files/pdf/{token}/{name} — публичная ссылка; доступ даёт только действующая подпись.
func (h *OrderHandler) ServePDF(w http.ResponseWriter, r *http.Request) {
	id, err := h.pdfService.VerifyLink(chi.URLParam(r, "token"))
	if err != nil {
		http.Error(w, "ссылка недействительна или устарела, запросите новую в приложении", http.StatusGone)
		return
	}
	doc, err := h.pdfService.BuildOrderPDF(r.Context(), id)
	if err != nil {
		writePDFError(w, err)
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	servePDF(w, r, doc, id, disposition)
}

// POST /api/v1/orders/{id}/pdf/send — бот присылает накладную в личный чат пользователю.
func (h *OrderHandler) PDFSend(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(r)
	if !ok {
		http.Error(w, "некорректный ID заказа", http.StatusBadRequest)
		return
	}
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "ошибка авторизации", http.StatusUnauthorized)
		return
	}
	doc, err := h.pdfService.BuildOrderPDF(r.Context(), id)
	if err != nil {
		writePDFError(w, err)
		return
	}
	caption := fmt.Sprintf("📄 Накладная к заказу №%d", id)
	if err := h.botService.SendDocument(r.Context(), user.UserID, doc.Filename, doc.Data, caption); err != nil {
		if errors.Is(err, service.ErrBotCannotWrite) || errors.Is(err, service.ErrNoTelegramAccount) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		log.Printf("PDF send (order %d, user %d): %v", id, user.UserID, err)
		http.Error(w, "не удалось отправить файл в Telegram", http.StatusBadGateway)
		return
	}
	pdfWriteJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

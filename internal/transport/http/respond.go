package http

import (
	"errors"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"telegram-mini-app/internal/repository"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
)

var fieldLabels = map[string]string{
	"SupplierID": "поставщик", "CarrierID": "перевозчик", "TruckNumber": "номер машины", "ArrivalDate": "дата прихода",
	"Items": "позиции", "Species": "порода", "LengthMM": "длина", "DiameterMM": "диаметр", "Count": "количество",
	"VolumeM3": "объём", "Logs": "сырьё", "Boards": "доски", "LogID": "сырьё", "Date": "дата", "ContractorID": "клиент",
	"BoardID": "доска", "Price": "цена", "HeightMM": "толщина", "WidthMM": "ширина", "Grade": "сорт",
	"DeliveryPrice": "доставка", "ExtraPrice": "наценка", "Notes": "примечание", "PricePerM3": "цена за м³",
}

// validationMessage превращает ошибку валидатора в понятную человеку фразу
// (вместо «Key: 'CreateOrderRequest.Items[0].Count' Error:Field validation...»).
func validationMessage(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) || len(ve) == 0 {
		return "некорректные данные"
	}
	fe := ve[0]
	name := fieldLabels[fe.Field()]
	if name == "" {
		name = fe.Field()
	}
	switch fe.Tag() {
	case "required":
		return "не заполнено поле «" + name + "»"
	case "gt":
		return "поле «" + name + "» должно быть больше " + fe.Param()
	case "gte":
		return "поле «" + name + "» не может быть меньше " + fe.Param()
	case "min":
		if fe.Kind() == reflect.Slice {
			return "добавьте хотя бы одну позицию («" + name + "»)"
		}
		return "поле «" + name + "» слишком короткое"
	case "max":
		return "поле «" + name + "» слишком длинное (максимум " + fe.Param() + ")"
	}
	return "некорректное значение поля «" + name + "»"
}

// parseDate проверяет дату формата ГГГГ-ММ-ДД: иначе БД вернула бы ошибку 500.
func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, errors.New("дата должна быть в формате ГГГГ-ММ-ДД")
	}
	if y := t.Year(); y < 2000 || y > 2100 {
		return time.Time{}, errors.New("дата вне допустимого диапазона")
	}
	return t, nil
}

// pageParams читает limit/offset и ограничивает размер выдачи.
func pageParams(r *http.Request, defLimit, maxLimit int) (limit, offset int) {
	limit = defLimit
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o > 0 {
		offset = o
	}
	return limit, offset
}

// serverError пишет подробности в лог, а пользователю отдаёт короткую фразу без внутренностей (SQL, имена таблиц).
func serverError(w http.ResponseWriter, r *http.Request, what string, err error) {
	log.Printf("ERROR %s %s: %s: %v", r.Method, r.URL.Path, what, err)
	http.Error(w, what+", попробуйте ещё раз", http.StatusInternalServerError)
}

// respondError выбирает код и текст по типу ошибки: ожидаемые ситуации (нет товара, неверная ссылка на запись)
// получают понятное сообщение и код 4xx, остальное считается сбоем сервера.
func respondError(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case errors.Is(err, repository.ErrInsufficientStock):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, repository.ErrBoardNotFound), errors.Is(err, repository.ErrBoardEmpty):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503": // foreign_key_violation
			http.Error(w, "указанная запись не найдена (поставщик, перевозчик, клиент, доска или сырьё)", http.StatusBadRequest)
			return
		case "23505": // unique_violation
			http.Error(w, "такая запись уже существует", http.StatusConflict)
			return
		case "23514": // check_violation
			http.Error(w, "значение вне допустимого диапазона", http.StatusBadRequest)
			return
		case "23502": // not_null_violation
			http.Error(w, "не заполнено обязательное поле", http.StatusBadRequest)
			return
		case "22007", "22008", "22P02", "22003": // неверная дата, число или диапазон
			http.Error(w, "некорректное значение (дата или число)", http.StatusBadRequest)
			return
		}
	}
	serverError(w, r, what, err)
}
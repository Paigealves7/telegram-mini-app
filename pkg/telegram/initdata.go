package telegram

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "net/url"
    "sort"
    "strings"
    "log"
)

type TGUser struct {
    ID        int64  `json:"id"`
    FirstName string `json:"first_name"`
    LastName  string `json:"last_name,omitempty"`
    Username  string `json:"username,omitempty"`
}

func ValidateInitData(initDataRaw, botToken string) (*TGUser, error) {
    // 1. Чистим токен
    cleanToken := strings.TrimSpace(botToken)
    cleanToken = strings.Trim(cleanToken, "\"'")

    // 2. Парсим БЕЗ декодирования
    pairs := strings.Split(initDataRaw, "&")
    values := make(map[string]string)
    for _, pair := range pairs {
        idx := strings.Index(pair, "=")
        if idx == -1 {
            continue
        }
        values[pair[:idx]] = pair[idx+1:]
    }

    // 3. hash
    hash := values["hash"]
    if hash == "" {
        return nil, errors.New("hash missing from initData")
    }

    // 4. Убираем hash И signature
    delete(values, "hash")
    delete(values, "signature")

    // 5. Сортируем ключи
    keys := make([]string, 0, len(values))
    for k := range values {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    // 6. data_check_string из СЫРЫХ значений
    var parts []string
    for _, k := range keys {
        parts = append(parts, k+"="+values[k])
    }
    dataCheckString := strings.Join(parts, "\n")

    // 7. secret_key = HMAC_SHA256("WebAppData", botToken)
    secretMac := hmac.New(sha256.New, []byte("WebAppData"))
    secretMac.Write([]byte(cleanToken))
    secretKey := secretMac.Sum(nil)

    // 8. calculated = HMAC_SHA256(secretKey, dataCheckString)
    dataMac := hmac.New(sha256.New, secretKey)
    dataMac.Write([]byte(dataCheckString))
    calculatedHash := hex.EncodeToString(dataMac.Sum(nil))

    // 9. Сравниваем
    if calculatedHash != hash {
        log.Printf("DEBUG dataCheckString: %q", dataCheckString)
        log.Printf("DEBUG calculatedHash: %s", calculatedHash)
        log.Printf("DEBUG receivedHash: %s", hash)
        return nil, errors.New("invalid hash signature")
    }

    // 10. Декодируем user
    userDecoded, err := url.QueryUnescape(values["user"])
    if err != nil {
        return nil, errors.New("failed to unescape user")
    }

    var user TGUser
    if err := json.Unmarshal([]byte(userDecoded), &user); err != nil {
        return nil, errors.New("failed to parse user")
    }

    return &user, nil
}

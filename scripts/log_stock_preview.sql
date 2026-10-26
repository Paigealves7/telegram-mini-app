-- Просмотр ДО миграции: что станет остатком каждой партии сырья.
-- Запуск: docker exec -i sawmill_db psql -U postgres -d telegram-mini-app_db < scripts/log_stock_preview.sql

-- 1. Остаток по партиям (отрицательный остаток = за всё время распилили больше, чем пришло; он обнулится)
SELECT li.id AS "партия", li.species AS "порода", li.volume_m3 AS "пришло_м3",
       COALESCE(SUM(sl.volume_m3), 0) AS "распилено_м3",
       li.volume_m3 - COALESCE(SUM(sl.volume_m3), 0) AS "остаток_м3"
FROM log_items li
LEFT JOIN sawed_logs sl ON sl.log_id = li.id
GROUP BY li.id
ORDER BY 5, li.id;

-- 2. Распилы без привязки к партии (старые записи): они не уменьшат ни одну партию
SELECT COUNT(*) AS "записей", COALESCE(SUM(volume_m3), 0) AS "объём_м3" FROM sawed_logs WHERE log_id IS NULL;
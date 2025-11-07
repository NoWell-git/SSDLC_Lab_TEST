package main

import (
        "bufio"
        "database/sql"
        "fmt"
        "os"
        "strings"
        "time"

        _ "github.com/jackc/pgx/v5/stdlib"
        "github.com/joho/godotenv"
        "golang.org/x/term"
)

type Config struct {
        Host           string
        Port           string
        Database       string
        User           string
        Password       string
        SSLMode        string
        LogFile        string
        RequestTimeout string
}

var db *sql.DB
var logFile *os.File

// Белые списки колонок для каждой таблицы
var whiteLists = map[string][]string{
        "categories":    {"id", "name"},
        "manufacturers": {"id", "name", "country"},
        "components":    {"id", "name", "category_id", "manufacturer_id", "model", "price", "release_year"},
        "stock":         {"id", "component_id", "quantity", "warehouse_location"},
}

func loadConfig() (Config, error) {
        godotenv.Load("config.env")
        return Config{
                Host:           os.Getenv("DB_HOST"),
                Port:           os.Getenv("DB_PORT"),
                Database:       os.Getenv("DB_NAME"),
                SSLMode:        os.Getenv("DB_SSLMODE"),
                LogFile:        os.Getenv("LOG_FILE"),
                RequestTimeout: os.Getenv("REQUEST_TIMEOUT"),
        }, nil
}

func initLog() {
        if os.Getenv("LOG_FILE") != "" {
                var err error
                logFile, err = os.OpenFile(os.Getenv("LOG_FILE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
                if err != nil {
                        fmt.Println("Не удалось открыть лог-файл, логи только в консоль")
                }
        }
}

func logInfo(msg string) {
        fmt.Println(msg)
        if logFile != nil {
                fmt.Fprintf(logFile, "[%s] INFO: %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
        }
}

func logError(msg string) {
        fmt.Fprintf(os.Stderr, "ОШИБКА: %s\n", msg)
        if logFile != nil {
			    fmt.Fprintf(logFile, "[%s] ERROR: %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
        }
}

func connectDB(cfg Config) error {
        connStr := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
                cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password, cfg.SSLMode)
        var err error
        db, err = sql.Open("pgx", connStr)
        if err != nil {
                return err
        }
        db.SetMaxOpenConns(10)
        return db.Ping()
}

func isValidColumn(table, col string) bool {
        cols, ok := whiteLists[table]
        if !ok {
                return false
        }
        for _, c := range cols {
                if c == col {
                        return true
                }
        }
        return false
}

func isValidTable(table string) bool {
        _, ok := whiteLists[table]
        return ok
}

// 6.1.1 — просмотр без фильтрации
func viewAll(table string) {
        if !isValidTable(table) {
                logError("Недопустимая таблица")
                return
        }
        rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", table))
        if err != nil {
                logError("Не удалось получить данные")
                return
        }
        defer rows.Close()
        printRows(rows)
}

// 6.1.2 — фильтрация по одному полю
func viewFilteredOne(table string) {
        if !isValidTable(table) {
                logError("Недопустимая таблица")
                return
        }
        reader := bufio.NewReader(os.Stdin)
        fmt.Print("Колонка для фильтрации: ")
        col, _ := reader.ReadString('\n')
        col = strings.TrimSpace(col)
        if !isValidColumn(table, col) {
                logError("Недопустимая колонка")
                return
        }
        fmt.Print("Значение: ")
        val, _ := reader.ReadString('\n')
        val = strings.TrimSpace(val)
        query := fmt.Sprintf("SELECT * FROM %s WHERE %s = $1", table, col)
        rows, err := db.Query(query, val)
        if err != nil {
                logError("Ошибка фильтрации")
                return
        }
        defer rows.Close()
        printRows(rows)
}

// 6.1.3 — фильтрация по нескольким полям
func viewFilteredMultiple(table string) {
        if !isValidTable(table) {
                logError("Недопустимая таблица")
                return
        }
        reader := bufio.NewReader(os.Stdin)
        whereParts := []string{}
        args := []interface{}{}
        argIndex := 1
        fmt.Println("Введите колонки и значения для фильтрации (пустая строка — завершить):")
        for {
                fmt.Print("Колонка: ")
                col, _ := reader.ReadString('\n')
                col = strings.TrimSpace(col)
                if col == "" {
                        break
                }
                if !isValidColumn(table, col) {
                        logError("Недопустимая колонка")
                        continue
                }
                fmt.Print("Значение: ")
                val, _ := reader.ReadString('\n')
                val = strings.TrimSpace(val)
                whereParts = append(whereParts, fmt.Sprintf("%s = $%d", col, argIndex))
                args = append(args, val)
                argIndex++
        }
        if len(whereParts) == 0 {
                logError("Нет условий для фильтрации")
                return
        }
        query := fmt.Sprintf("SELECT * FROM %s WHERE %s", table, strings.Join(whereParts, " AND "))
        rows, err := db.Query(query, args...)
        if err != nil {
                logError("Ошибка фильтрации")
                return
        }
        defer rows.Close()
        printRows(rows)
}

// 6.2.1 — обновление одной записи
func updateSingle(table string) {
        reader := bufio.NewReader(os.Stdin)
        fmt.Print("ID записи: ")
        idStr, _ := reader.ReadString('\n')
        var id int
        fmt.Sscanf(idStr, "%d", &id)
        setParts := []string{}
        args := []interface{}{}
        argIndex := 1
        fmt.Println("Введите колонки и значения (пустая строка — завершить):")
        for {
                fmt.Print("Колонка: ")
                col, _ := reader.ReadString('\n')
                col = strings.TrimSpace(col)
                if col == "" {
                        break
                }
                if !isValidColumn(table, col) || col == "id" {
                        fmt.Println("Недопустимая колонка")
                        continue
                }
                fmt.Print("Новое значение: ")
                val, _ := reader.ReadString('\n')
                val = strings.TrimSpace(val)
                setParts = append(setParts, fmt.Sprintf("%s = $%d", col, argIndex))
                args = append(args, val)
                argIndex++
        }
        if len(setParts) == 0 {
                fmt.Println("Ничего не обновлено")
                return
        }
        args = append(args, id)
        query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", table, strings.Join(setParts, ", "), argIndex)
        _, err := db.Exec(query, args...)
        if err != nil {
                logError("Не удалось обновить запись")
        } else {
                logInfo("Запись успешно обновлена")
        }
}

// 6.2.2 — обновление нескольких записей по IN
func updateMultiple(table string) {
        reader := bufio.NewReader(os.Stdin)
        fmt.Print("Колонка для обновления: ")
        updateCol, _ := reader.ReadString('\n')
        updateCol = strings.TrimSpace(updateCol)
        if !isValidColumn(table, updateCol) || updateCol == "id" {
                logError("Недопустимая колонка для обновления")
                return
        }
        fmt.Print("Новое значение: ")
        newVal, _ := reader.ReadString('\n')
        newVal = strings.TrimSpace(newVal)
        fmt.Print("Колонка для фильтрации (IN): ")
        filterCol, _ := reader.ReadString('\n')
        filterCol = strings.TrimSpace(filterCol)
        if !isValidColumn(table, filterCol) {
                logError("Недопустимая колонка для фильтрации")
                return
        }
        fmt.Print("Значения для IN (через запятую): ")
        valuesStr, _ := reader.ReadString('\n')
        valuesStr = strings.TrimSpace(valuesStr)
        values := strings.Split(valuesStr, ",")
        args := []interface{}{newVal}
        placeholders := []string{}
        for i, val := range values {
                placeholders = append(placeholders, fmt.Sprintf("$%d", i+2))
                args = append(args, strings.TrimSpace(val))
        }
        if len(placeholders) == 0 {
                logError("Нет значений для фильтрации")
                return
        }
        query := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s IN (%s)", table, updateCol, filterCol, strings.Join(placeholders, ", "))
        _, err := db.Exec(query, args...)
        if err != nil {
                logError("Не удалось обновить записи")
        } else {
                logInfo("Записи успешно обновлены")
        }
}

// 6.3.1 — вставка одной строки
func insertSingle(table string) {
        if !isValidTable(table) {
                logError("Недопустимая таблица")
                return
        }
        reader := bufio.NewReader(os.Stdin)
        cols := whiteLists[table]
        cols = cols[1:] // убираем id
        values := []interface{}{}
        placeholders := []string{}
        selectedCols := []string{}
        argIndex := 1
        for _, col := range cols {
                fmt.Printf("%s: ", col)
                val, _ := reader.ReadString('\n')
                val = strings.TrimSpace(val)
                if val == "" {
                        continue // Пропуск необязательных
                }
                selectedCols = append(selectedCols, col)
                values = append(values, val)
                placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
                argIndex++
        }
        if len(placeholders) == 0 {
                logError("Нет данных для вставки")
                return
        }
        query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
                table, strings.Join(selectedCols, ", "), strings.Join(placeholders, ", "))
        _, err := db.Exec(query, values...)
        if err != nil {
                logError("Ошибка вставки")
        } else {
                logInfo("Запись успешно добавлена")
        }
}

// 6.4.1 — вставка нескольких строк в одну таблицу
func insertMultipleSingle(table string) {
        if !isValidTable(table) {
                logError("Недопустимая таблица")
                return
        }
        reader := bufio.NewReader(os.Stdin)
        cols := whiteLists[table]
        cols = cols[1:] // убираем id
        numRows := 0
        fmt.Print("Количество строк для вставки: ")
        fmt.Scanf("%d\n", &numRows)
        if numRows <= 0 {
                logError("Неверное количество")
                return
        }
        values := []interface{}{}
        placeholders := []string{}
        argIndex := 1
        for i := 0; i < numRows; i++ {
                rowPlaceholders := []string{}
                fmt.Printf("Строка %d:\n", i+1)
                for _, col := range cols {
                        fmt.Printf("%s: ", col)
                        val, _ := reader.ReadString('\n')
                        val = strings.TrimSpace(val)
                        if val == "" {
                                continue
                        }
                        rowPlaceholders = append(rowPlaceholders, fmt.Sprintf("$%d", argIndex))
                        values = append(values, val)
                        argIndex++
                }
                if len(rowPlaceholders) > 0 {
                        placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(rowPlaceholders, ", ")))
                }
        }
        if len(placeholders) == 0 {
                logError("Нет данных для вставки")
                return
        }
        query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
                table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
        _, err := db.Exec(query, values...)
        if err != nil {
                logError("Ошибка вставки")
        } else {
                logInfo("Строки успешно добавлены")
        }
}

// 6.3.2 / 6.4.2 — вставка в связанные таблицы (одна или несколько строк)
func insertRelated(multiple bool) {
        reader := bufio.NewReader(os.Stdin)
        numRows := 1
        if multiple {
                fmt.Print("Количество наборов для вставки: ")
                fmt.Scanf("%d\n", &numRows)
                if numRows <= 0 {
                        logError("Неверное количество")
                        return
                }
        }
        tx, err := db.Begin()
        if err != nil {
                logError("Не удалось начать транзакцию")
                return
        }
        for i := 0; i < numRows; i++ {
                var compID int
                fmt.Printf("=== Добавление компонента %d ===\n", i+1)
                fmt.Print("name: ")
                name, _ := reader.ReadString('\n')
                name = strings.TrimSpace(name)
                fmt.Print("category_id: ")
                catID, _ := reader.ReadString('\n')
                catID = strings.TrimSpace(catID)
                fmt.Print("manufacturer_id: ")
                manID, _ := reader.ReadString('\n')
                manID = strings.TrimSpace(manID)
                fmt.Print("model: ")
                model, _ := reader.ReadString('\n')
                model = strings.TrimSpace(model)
                fmt.Print("price: ")
                price, _ := reader.ReadString('\n')
                price = strings.TrimSpace(price)
                fmt.Print("release_year: ")
                year, _ := reader.ReadString('\n')
                year = strings.TrimSpace(year)
                err = tx.QueryRow(`
                        INSERT INTO components (name, category_id, manufacturer_id, model, price, release_year)
                        VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
                        name, catID, manID, model, price, year).Scan(&compID)
                if err != nil {
                        tx.Rollback()
                        logError("Ошибка вставки компонента")
                        return
                }
                fmt.Print("quantity на складе: ")
                quantity, _ := reader.ReadString('\n')
                quantity = strings.TrimSpace(quantity)
                fmt.Print("warehouse_location: ")
                location, _ := reader.ReadString('\n')
                location = strings.TrimSpace(location)
                _, err = tx.Exec(`INSERT INTO stock (component_id, quantity, warehouse_location) VALUES ($1,$2,$3)`,
                        compID, quantity, location)
                if err != nil {
                        tx.Rollback()
                        logError("Ошибка добавления на склад")
                        return
                }
        }
        err = tx.Commit()
        if err != nil {
                logError("Ошибка коммита транзакции")
        } else {
                logInfo("Компоненты и записи на склад добавлены")
        }
}

// Красивая печать результата SELECT
func printRows(rows *sql.Rows) {
        cols, _ := rows.Columns()
        var records [][]interface{}
        for rows.Next() {
                values := make([]interface{}, len(cols))
                valuePtrs := make([]interface{}, len(cols))
                for i := range values {
                        valuePtrs[i] = &values[i]
                }
                rows.Scan(valuePtrs...)
                records = append(records, values)
        }
        // Заголовки
        for _, col := range cols {
                fmt.Printf("%-20s", col)
        }
        fmt.Println("\n" + strings.Repeat("-", 20*len(cols)))
        // Данные
        for _, record := range records {
                for _, val := range record {
                        if val == nil {
                                fmt.Printf("%-20s", "<NULL>")
                        } else {
                                fmt.Printf("%-20v", val)
                        }
                }
                fmt.Println()
        }
        fmt.Println()
}

func main() {
        cfg, _ := loadConfig()
        initLog()
        // Ввод логина/пароля
        fmt.Print("Логин: ")
        userInput, _ := bufio.NewReader(os.Stdin).ReadString('\n')
        cfg.User = strings.TrimSpace(userInput)
        fmt.Print("Пароль: ")
        pass, _ := term.ReadPassword(int(os.Stdin.Fd()))
        fmt.Println()
        cfg.Password = string(pass)
        if err := connectDB(cfg); err != nil {
                logError("Не удалось подключиться к базе данных")
                return
        }
        logInfo("Подключение к БД успешно")
        reader := bufio.NewReader(os.Stdin)
        for {
                fmt.Println("\n=== МЕНЮ ===")
                fmt.Println("1. Просмотр таблицы (без фильтра)")
                fmt.Println("2. Фильтрация по одному полю")
                fmt.Println("3. Фильтрация по нескольким полям")
                fmt.Println("4. Обновить одну запись")
                fmt.Println("5. Обновить несколько записей")
                fmt.Println("6. Добавить в одну таблицу (одна строка)")
                fmt.Println("7. Добавить в одну таблицу (несколько строк)")
                fmt.Println("8. Добавить в связанные таблицы (одна)")
                fmt.Println("9. Добавить в связанные таблицы (несколько)")
                fmt.Println("0. Выход")
                fmt.Print("> ")
                choice, _ := reader.ReadString('\n')
                choice = strings.TrimSpace(choice)
                if choice == "0" {
                        break
                }
                var table string
                switch choice {
                case "1", "2", "3", "4", "5", "6", "7":
                        fmt.Print("Таблица (categories/manufacturers/components/stock): ")
                        table, _ = reader.ReadString('\n')
                        table = strings.TrimSpace(table)
                }
                switch choice {
                case "1":
                        viewAll(table)
                case "2":
                        viewFilteredOne(table)
                case "3":
                        viewFilteredMultiple(table)
                case "4":
                        updateSingle(table)
                case "5":
                        updateMultiple(table)
                case "6":
                        insertSingle(table)
                case "7":
                        insertMultipleSingle(table)
                case "8":
                        insertRelated(false)
                case "9":
                        insertRelated(true)
                default:
                        fmt.Println("Неверный выбор")
                }
        }
}

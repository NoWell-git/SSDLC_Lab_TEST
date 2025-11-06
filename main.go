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
		} else {
			defer logFile.Close()
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

// === ОСНОВНЫЕ ФУНКЦИИ ПО ТЗ ===

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
	argIndex := 1

	for _, col := range cols {
		fmt.Printf("%s: ", col)
		val, _ := reader.ReadString('\n')
		val = strings.TrimSpace(val)
		if val == "" {
			fmt.Println("Пропуск необязательного поля")
			continue
		}
		values = append(values, val)
		placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
		argIndex++
	}

	if len(placeholders) == 0 {
		logError("Нет данных для вставки")
		return
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(cols[:len(placeholders)], ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(query, values...)
	if err != nil {
		logError("Ошибка вставки")
	} else {
		logCarlos("Запись добавлена")
	}
}

func insertRelated() {
	tx, err := db.Begin()
	if err != nil {
		logError("Не удалось начать транзакцию")
		return
	}
	reader := bufio.NewReader(os.Stdin)

	// Вставка в components
	fmt.Println("=== Добавление компонента ===")
	var compID int
	err = tx.QueryRow(`
		INSERT INTO components (name, category_id, manufacturer_id, model, price, release_year) 
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		"RTX 5090", 2, 3, "FOUNDERS", 2500.00, 2025).Scan(&compID)
	if err != nil {
		tx.Rollback()
		logError("Ошибка вставки компонента")
		return
	}

	// Вставка в stock
	_, err = tx.Exec(`INSERT INTO stock (component_id, quantity, warehouse_location) VALUES ($1,$2,$3)`,
		compID, 10, "X1")
	if err != nil {
		tx.Rollback()
		logError("Ошибка добавления на склад")
		return
	}

	tx.Commit()
	logInfo(fmt.Sprintf("Компонент и склад добавлены! ID компонента: %d", compID))
}

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

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

	// Печать заголовков
	for _, col := range cols {
		fmt.Printf("%-20s", col)
	}
	fmt.Println("\n" + strings.Repeat("-", 20*len(cols)))

	// Печать строк
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

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Логин: ")
	user, _ := reader.ReadString('\n')
	cfg.User = strings.TrimSpace(user)
	fmt.Print("Пароль: ")
	pass, _ := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	cfg.Password = string(pass)

	if err := connectDB(cfg); err != nil {
		logError("Не удалось подключиться к базе данных")
		return
	}
	logInfo("Подключение к БД успешно")

	for {
		fmt.Println("\n=== МЕНЮ ===")
		fmt.Println("1. Просмотр таблицы")
		fmt.Println("2. Фильтрация по одному полю")
		fmt.Println("3. Обновить запись")
		fmt.Println("4. Добавить в одну таблицу")
		fmt.Println("5. Добавить в связанные таблицы")
		fmt.Println("0. Выход")
		fmt.Print("> ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "0" {
			break
		}

		if strings.HasPrefix(choice, "1") {
			fmt.Print("Таблица (categories/manufacturers/components/stock): ")
			table, _ := reader.ReadString('\n')
			table = strings.TrimSpace(table)
			viewAll(table)
		}
		if strings.HasPrefix(choice, "2") {
			fmt.Print("Таблица: ")
			table, _ := reader.ReadString('\n')
			table = strings.TrimSpace(table)
			viewFilteredOne(table)
		}
		if strings.HasPrefix(choice, "3") {
			fmt.Print("Таблица: ")
			table, _ := reader.ReadString('\n')
			table = strings.TrimSpace(table)
			updateSingle(table)
		}
		if strings.HasPrefix(choice, "4") {
			fmt.Print("Таблица: ")
			table, _ := reader.ReadString('\n')
			table = strings.TrimSpace(table)
			insertSingle(table)
		}
		if strings.HasPrefix(choice, "5") {
			insertRelated()
		}
	}
}

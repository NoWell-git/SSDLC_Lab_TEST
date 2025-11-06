package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Config — структура конфигурации из config.yaml
type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
}

var db     *sql.DB
var logger *log.Logger

// Белые списки таблиц и колонок
var validTables = map[string]bool{
	"categories":     true,
	"manufacturers":  true,
	"components":     true,
	"compatibility":  true,
}

var validColumns = map[string]map[string]bool{
	"categories": {
		"id": true, "name": true, "description": true,
	},
	"manufacturers": {
		"id": true, "name": true, "country": true,
	},
	"components": {
		"id": true, "name": true, "category_id": true, "manufacturer_id": true,
		"model": true, "price": true, "release_year": true, "in_stock": true,
	},
	"compatibility": {
		"cpu_id": true, "motherboard_id": true, "socket": true,
	},
}

// Инициализация логгера (stdout + файл из LOG_FILE)
func initLogger() {
	logPath := os.Getenv("LOG_FILE")
	writer := io.Writer(os.Stdout)
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			writer = io.MultiWriter(os.Stdout, f)
		}
	}
	logger = log.New(writer, "[PC-APP] ", log.LstdFlags|log.Lmsgprefix)
}

// Загрузка config.yaml
func loadConfig() Config {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		logger.Fatal("Не найден config.yaml")
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		logger.Fatal("Ошибка парсинга config.yaml:", err)
	}
	return cfg
}

// Подключение к БД
func connect(cfg Config) error {
	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password, cfg.SSLMode)
	var err error
	db, err = sql.Open("pgx", connStr)
	if err != nil {
		return err
	}
	return db.Ping()
}

// Безопасный ввод строки
func readLine(r *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

// Проверка таблицы
func isValidTable(table string) bool {
	return validTables[table]
}

// Проверка колонки
func isValidColumn(table, col string) bool {
	if cols, ok := validColumns[table]; ok {
		return cols[col]
	}
	return false
}

// Вывод результата запроса
func printRows(rows *sql.Rows) error {
	columns, _ := rows.Columns()
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				fmt.Printf("%s: %s  ", col, string(b))
			} else {
				fmt.Printf("%s: %v  ", col, val)
			}
		}
		fmt.Println()
	}
	return rows.Err()
}

// ======================= МЕНЮ =======================
func main() {
	initLogger()
	cfg := loadConfig()
	reader := bufio.NewReader(os.Stdin)

	// Ввод учётных данных
	cfg.User = readLine(reader, "Логин: ")
	pass, _ := term.ReadPassword(int(os.Stdin.Fd()))
	cfg.Password = string(pass)
	fmt.Println()

	if err := connect(cfg); err != nil {
		fmt.Println("Ошибка подключения к БД. Проверьте данные.")
		logger.Println("CONNECT FAIL:", err)
		return
	}
	fmt.Println("Подключение к БД успешно!")
	logger.Println("CONNECT OK user:", cfg.User)

	for {
		fmt.Println("\n=== ГЛАВНОЕ МЕНЮ ===")
		fmt.Println("1. Просмотр таблиц")
		fmt.Println("2. Обновление записей")
		fmt.Println("3. Добавление записей")
		fmt.Println("4. Выход")
		choice := readLine(reader, "Выбор: ")

		switch choice {
		case "1":
			viewMenu(reader)
		case "2":
			updateMenu(reader)
		case "3":
			insertMenu(reader)
		case "4":
			fmt.Println("До свидания!")
			return
		default:
			fmt.Println("Неверный выбор")
		}
	}
}

// ======================= ПРОСМОТР =======================
func viewMenu(r *bufio.Reader) {
	table := readLine(r, "Таблица (categories/manufacturers/components/compatibility): ")
	if !isValidTable(table) {
		fmt.Println("Недопустимая таблица")
		return
	}

	fmt.Println("1. Все записи")
	fmt.Println("2. Фильтр по одному полю")
	fmt.Println("3. Фильтр по нескольким полям")
	mode := readLine(r, "Режим: ")

	var query string
	var args []interface{}

	switch mode {
	case "1":
		query = fmt.Sprintf("SELECT * FROM %s", table)
	case "2":
		col := askColumn(r, table)
		val := readLine(r, "Значение: ")
		query = fmt.Sprintf("SELECT * FROM %s WHERE %s = $1", table, col)
		args = append(args, val)
	case "3":
		query = fmt.Sprintf("SELECT * FROM %s WHERE ", table)
		fmt.Print("Количество условий: ")
		nStr, _ := r.ReadString('\n')
		n, _ := strconv.Atoi(strings.TrimSpace(nStr))
		if n < 1 {
			n = 1
		}
		if n > 5 {
			n = 5
		}
		for i := 0; i < n; i++ {
			col := askColumn(r, table)
			val := readLine(r, fmt.Sprintf("Значение для %s: ", col))
			if i > 0 {
				query += " AND "
			}
			query += fmt.Sprintf("%s = $%d", col, i+1)
			args = append(args, val)
		}
	default:
		return
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Println("Ошибка выполнения запроса")
		logger.Println("VIEW ERR:", err, query)
		return
	}
	defer rows.Close()
	if err := printRows(rows); err != nil {
		logger.Println("PRINT ERR:", err)
	}
}

// ======================= ОБНОВЛЕНИЕ =======================
func updateMenu(r *bufio.Reader) {
	table := readLine(r, "Таблица: ")
	if !isValidTable(table) {
		fmt.Println("Недопустимая таблица")
		return
	}

	fmt.Println("1. Обновить одну запись (по id)")
	fmt.Println("2. Обновить несколько записей (по IN)")
	mode := readLine(r, "Режим: ")

	if mode == "1" {
		updateSingle(r, table)
	} else {
		updateMultiple(r, table)
	}
}

func updateSingle(r *bufio.Reader, table string) {
	id := readLine(r, "ID записи: ")
	query := fmt.Sprintf("UPDATE %s SET ", table)
	var sets []string
	var args []interface{}
	idx := 1

	fmt.Println("Колонки для изменения (пустая строка — завершить):")
	for {
		col := readLine(r, "Колонка: ")
		if col == "" {
			break
		}
		if !isValidColumn(table, col) || col == "id" {
			fmt.Println("Недопустимо")
			continue
		}
		val := readLine(r, "Новое значение: ")
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, val)
		idx++
	}
	if len(sets) == 0 {
		fmt.Println("Нечего обновлять")
		return
	}
	query += strings.Join(sets, ", ") + fmt.Sprintf(" WHERE id = $%d RETURNING id", idx)
	args = append(args, id)

	var newID int
	err := db.QueryRow(query, args...).Scan(&newID)
	if err != nil {
		fmt.Println("Ошибка обновления")
		logger.Println("UPDATE SINGLE ERR:", err)
		return
	}
	fmt.Printf("Запись id=%d обновлена\n", newID)
	logger.Printf("UPDATE SINGLE table=%s id=%d", table, newID)
}

func updateMultiple(r *bufio.Reader, table string) {
	col := askColumn(r, table)
	val := readLine(r, "Новое значение для "+col+": ")
	ids := readLine(r, "ID через запятую (1,2,3): ")
	query := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = ANY($2)", table, col)
	var idArr []int
	for _, s := range strings.Split(ids, ",") {
		if i, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			idArr = append(idArr, i)
		}
	}
	if len(idArr) == 0 {
		fmt.Println("Нет валидных ID")
		return
	}

	res, err := db.Exec(query, val, idArr)
	if err != nil {
		fmt.Println("Ошибка")
		logger.Println("UPDATE MULTI ERR:", err)
		return
	}
	count, _ := res.RowsAffected()
	fmt.Printf("Обновлено записей: %d\n", count)
	logger.Printf("UPDATE MULTI table=%s count=%d", table, count)
}

// ======================= ДОБАВЛЕНИЕ =======================
func insertMenu(r *bufio.Reader) {
	fmt.Println("1. Одну запись в одну таблицу")
	fmt.Println("2. Несколько записей в одну таблицу")
	fmt.Println("3. Запись в связанные таблицы (components + compatibility)")
	mode := readLine(r, "Режим: ")

	switch mode {
	case "1":
		insertSingle(r)
	case "2":
		insertBulk(r)
	case "3":
		insertWithRelations(r)
	default:
		fmt.Println("Неверный режим")
	}
}

func insertSingle(r *bufio.Reader) {
	table := readLine(r, "Таблица: ")
	if !isValidTable(table) {
		fmt.Println("Недопустимая")
		return
	}

	query := fmt.Sprintf("INSERT INTO %s (", table)
	var cols []string
	var placeholders []string
	var args []interface{}

	fmt.Println("Колонки и значения (пустая колонка — завершить):")
	idx := 1
	for {
		col := readLine(r, "Колонка: ")
		if col == "" {
			break
		}
		if !isValidColumn(table, col) || col == "id" {
			fmt.Println("Недопустимо")
			continue
		}
		val := readLine(r, "Значение: ")
		cols = append(cols, col)
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
		args = append(args, val)
		idx++
	}
	if len(cols) == 0 {
		return
	}
	query += strings.Join(cols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ") RETURNING id"

	var newID int
	err := db.QueryRow(query, args...).Scan(&newID)
	if err != nil {
		fmt.Println("Ошибка вставки")
		logger.Println("INSERT SINGLE ERR:", err)
		return
	}
	fmt.Printf("Создана запись id=%d\n", newID)
	logger.Printf("INSERT SINGLE table=%s id=%d", table, newID)
}

func insertBulk(r *bufio.Reader) {
	table := readLine(r, "Таблица: ")
	if !isValidTable(table) {
		return
	}

	var cols []string
	fmt.Print("Колонки через запятую: ")
	colsInput, _ := r.ReadString('\n')
	for _, c := range strings.Split(strings.TrimSpace(colsInput), ",") {
		c = strings.TrimSpace(c)
		if isValidColumn(table, c) && c != "id" {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", table, strings.Join(cols, ", "))
	var args []interface{}
	var values []string

	fmt.Println("Введите строки (пустая — завершить):")
	for {
		line := readLine(r, "Значения через запятую: ")
		if line == "" {
			break
		}
		parts := strings.Split(line, ",")
		if len(parts) != len(cols) {
			fmt.Println("Неверное количество значений")
			continue
		}
		ph := make([]string, len(cols))
		for i := range cols {
			ph[i] = fmt.Sprintf("$%d", len(args)+i+1)
			args = append(args, strings.TrimSpace(parts[i]))
		}
		values = append(values, "("+strings.Join(ph, ", ")+")")
	}
	if len(values) == 0 {
		return
	}
	query += strings.Join(values, ", ") + " RETURNING id"

	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Println("Ошибка bulk-вставки")
		logger.Println("INSERT BULK ERR:", err)
		return
	}
	defer rows.Close()
	fmt.Print("Созданные ID: ")
	for rows.Next() {
		var id int
		rows.Scan(&id)
		fmt.Printf("%d ", id)
	}
	fmt.Println()
}

// Вставка в связанные таблицы (6.4.2)
func insertWithRelations(r *bufio.Reader) {
	tx, err := db.Begin()
	if err != nil {
		fmt.Println("Ошибка транзакции")
		return
	}
	defer tx.Rollback()

	// 1. Добавляем CPU
	cpuName := readLine(r, "Название CPU: ")
	cpuModel := readLine(r, "Модель CPU: ")
	cpuPrice := readLine(r, "Цена CPU: ")
	cpuYear := readLine(r, "Год выпуска: ")

	var cpuID int
	err = tx.QueryRow(`
		INSERT INTO components (name, category_id, manufacturer_id, model, price, release_year, in_stock)
		VALUES ($1, 1, 1, $2, $3, $4, true) RETURNING id`,
		cpuName, cpuModel, cpuPrice, cpuYear).Scan(&cpuID)
	if err != nil {
		fmt.Println("Ошибка вставки CPU")
		logger.Println("INSERT CPU ERR:", err)
		return
	}

	// 2. Добавляем материнку
	mbName := readLine(r, "Название материнской платы: ")
	mbModel := readLine(r, "Модель: ")
	mbPrice := readLine(r, "Цена: ")

	var mbID int
	err = tx.QueryRow(`
		INSERT INTO components (name, category_id, manufacturer_id, model, price, release_year, in_stock)
		VALUES ($1, 2, 3, $2, $3, 2023, true) RETURNING id`,
		mbName, mbModel, mbPrice).Scan(&mbID)
	if err != nil {
		fmt.Println("Ошибка вставки MB")
		logger.Println("INSERT MB ERR:", err)
		return
	}

	// 3. Совместимость
	socket := readLine(r, "Сокет (например, AM5): ")
	_, err = tx.Exec(`
		INSERT INTO compatibility (cpu_id, motherboard_id, socket)
		VALUES ($1, $2, $3)`,
		cpuID, mbID, socket)
	if err != nil {
		fmt.Println("Ошибка совместимости")
		logger.Println("COMPAT ERR:", err)
		return
	}

	if err := tx.Commit(); err != nil {
		fmt.Println("Ошибка коммита")
		return
	}

	fmt.Printf("Успешно добавлено: CPU id=%d, MB id=%d, совместимость\n", cpuID, mbID)
	logger.Printf("INSERT RELATED cpu=%d mb=%d", cpuID, mbID)
}

// ======================= ВСПОМОГАТЕЛЬНЫЕ =======================
func askColumn(r *bufio.Reader, table string) string {
	for {
		col := readLine(r, "Колонка: ")
		if isValidColumn(table, col) {
			return col
		}
		fmt.Println("Недопустимая колонка. Доступно:", columnList(table))
	}
}

func columnList(table string) string {
	var list []string
	for col := range validColumns[table] {
		list = append(list, col)
	}
	return strings.Join(list, ", ")
}

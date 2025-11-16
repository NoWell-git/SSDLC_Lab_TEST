package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var tables = []string{"categories", "manufacturers", "components", "stock"}
var columns = map[string][]string{
	"categories":    {"id", "name", "description"},
	"manufacturers": {"id", "name", "country", "founded_year"},
	"components":    {"id", "name", "category_id", "manufacturer_id", "model", "price"},
	"stock":         {"id", "component_id", "quantity", "warehouse_location"},
}

type App struct {
	conn   *pgx.Conn
	logger *log.Logger
}

type Config struct {
	Host, Port, Database, User, Password, SSLMode, LogFile string
}

func main() {
	cfg, err := loadEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Неправильный логин или пароль.")
		os.Exit(1)
	}

	conn := connect(cfg)
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			logger = log.New(f, "", log.LstdFlags)
			defer f.Close()
		}
	}

	fmt.Println("Подключение к БД успешно!")
	(&App{conn, logger}).run()
}

func loadEnv() (Config, error) {
	godotenv.Load("config.env")
	user := read("Логин: ")
	password := read("Пароль: ")
	if user == "" || password == "" {
		return Config{}, fmt.Errorf("пустые данные")
	}
	return Config{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		Database: os.Getenv("DB_NAME"),
		User:     user,
		Password: password,
		SSLMode:  os.Getenv("DB_SSLMODE"),
		LogFile:  os.Getenv("LOG_FILE"),
	}, nil
}

func connect(cfg Config) *pgx.Conn {
	connStr := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password, cfg.SSLMode)
	conn, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("Некорректные данные подключения")
	}
	return conn
}

func (a *App) run() {
	for {
		printMenu()
		switch c := choice("> "); c {
		case "1":
			a.viewTable()
		case "2":
			a.filterTable()
		case "3":
			a.updateRecords()
		case "4":
			a.insertIntoTable()
		case "5":
			a.insertRelated()
		case "0":
			fmt.Println("Выход...")
			return
		default:
			fmt.Println("Неверный выбор")
		}
	}
}

func printMenu() {
	fmt.Println("\n=== МЕНЮ ===")
	fmt.Println("1. Просмотр таблицы")
	fmt.Println("2. Фильтрация")
	fmt.Println("3. Обновить запись")
	fmt.Println("4. Добавить запись")
	fmt.Println("5. Добавить в связанные таблицы")
	fmt.Println("0. Выход")
}

func read(prompt string) string {
	fmt.Print(prompt)
	s := strings.TrimSpace(scan())
	return s
}

func choice(prompt string) string {
	fmt.Print(prompt)
	return strings.TrimSpace(scan())
}

func scan() string {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return scanner.Text()
}

// === ВЫБОР ===
func selectOpt(prompt string, opts []string, back bool) string {
	for {
		if prompt != "" {
			fmt.Println(prompt)
		}
		for i, o := range opts {
			fmt.Printf("%d. %s\n", i+1, o)
		}
		if back {
			fmt.Println("0. Назад")
		}
		c := choice("> ")
		if c == "0" && back {
			return ""
		}
		if i, e := strconv.Atoi(c); e == nil && i > 0 && i <= len(opts) {
			return opts[i-1]
		}
		fmt.Println("Неверно")
	}
}

func (a *App) selectTable() string {
	return selectOpt("Таблица:", tables, true)
}

func (a *App) selectCol(t string) string {
	return selectOpt("Колонка:", columns[t], true)
}

func (a *App) selectUpdatableCol(t string) string {
	var opts []string
	for _, c := range columns[t] {
		if c != "id" && !strings.HasSuffix(c, "_id") {
			opts = append(opts, c)
		}
	}
	if len(opts) == 0 {
		fmt.Println("В этой таблице нет колонок, доступных для обновления")
		return ""
	}
	return selectOpt("Колонка для обновления:", opts, true)
}

func (a *App) selectMode() string {
	return selectOpt("", []string{"Одна", "Несколько"}, true)
}

// === 1. ПРОСМОТР ===
func (a *App) viewTable() {
	t := a.selectTable()
	if t == "" {
		return
	}
	a.query("SELECT * FROM "+t, nil)
	a.log("Просмотр: " + t)
}

// === 2. ФИЛЬТРАЦИЯ ===
func (a *App) filterTable() {
	m := a.selectMode()
	if m == "" {
		return
	}
	t := a.selectTable()
	if t == "" {
		return
	}

	var w []string
	var args []interface{}

	if m == "Одна" {
		c := a.selectCol(t)
		if c == "" {
			return
		}
		v := read(c + ": ")
		if v == "" {
			fmt.Println("Значение не может быть пустым")
			return
		}
		w = append(w, c+" = $1")
		args = append(args, v)
	} else {
		for {
			c := a.selectCol(t)
			if c == "" {
				break
			}
			v := read(c + ": ")
			if v == "" {
				fmt.Println("Значение не может быть пустым")
				continue
			}
			w = append(w, c+" = $"+strconv.Itoa(len(args)+1))
			args = append(args, v)
			if choice("Ещё? (y/n): ") != "y" {
				break
			}
		}
	}

	if len(w) == 0 {
		return
	}
	q := "SELECT * FROM " + t + " WHERE " + strings.Join(w, " AND ")
	a.query(q, args)
	a.log("Фильтр: " + q)
}

// === 3. ОБНОВЛЕНИЕ ===
func (a *App) updateRecords() {
	m := a.selectMode()
	if m == "" {
		return
	}
	t := a.selectTable()
	if t == "" {
		return
	}

	if m == "Одна" {
		id := read("ID: ")
		if id == "" {
			fmt.Println("ID не может быть пустым")
			return
		}
		u, args := a.updates(t)
		if len(u) == 0 {
			return
		}
		args = append(args, id)
		q := "UPDATE " + t + " SET " + strings.Join(u, ", ") + " WHERE id = $" + strconv.Itoa(len(args))
		a.exec(q, args)
		a.log("Обновление: " + q)
	} else {
		ids := read("ID (через запятую): ")
		c := a.selectUpdatableCol(t)
		if c == "" {
			return
		}
		v := read("Значение: ")
		if v == "" {
			fmt.Println("Значение не может быть пустым")
			return
		}
		idl := split(ids)
		in := make([]string, 0, len(idl))
		args := []interface{}{v}
		for i, id := range idl {
			if id == "" {
				continue
			}
			in = append(in, "$"+strconv.Itoa(i+2))
			args = append(args, id)
		}
		if len(in) == 0 {
			fmt.Println("Не указаны корректные ID")
			return
		}
		q := "UPDATE " + t + " SET " + c + " = $1 WHERE id IN (" + strings.Join(in, ",") + ")"
		a.exec(q, args)
		a.log("Массовое обновление: " + q)
	}
}

func (a *App) updates(t string) ([]string, []interface{}) {
	var u []string
	var args []interface{}
	for {
		c := a.selectUpdatableCol(t)
		if c == "" {
			break
		}
		v := read(c + ": ")
		if v == "" {
			fmt.Println("Значение не может быть пустым")
			continue
		}
		u = append(u, c+" = $"+strconv.Itoa(len(args)+1))
		args = append(args, v)
		if choice("Ещё? (y/n): ") != "y" {
			break
		}
	}
	return u, args
}

// === 4. ДОБАВЛЕНИЕ ЗАПИСИ ===
func (a *App) insertIntoTable() {
	m := a.selectMode()
	if m == "" {
		return
	}
	t := a.selectTable()
	if t == "" {
		return
	}
	if m == "Одна" {
		a.insertOne(t)
	} else {
		a.insertMany(t)
	}
}

func (a *App) insertOne(t string) {
	insertable := columns[t][1:] // кроме id
	var args []interface{}
	var placeholders []string
	for i, col := range insertable {
		val := strings.TrimSpace(read(col + ": "))
		if val == "" && col != "description" && col != "country" && col != "founded_year" && col != "model" && col != "warehouse_location" {
			fmt.Printf("Поле %s обязательно\n", col)
			return
		}
		args = append(args, val)
		placeholders = append(placeholders, "$"+strconv.Itoa(i+1))
	}
	q := `INSERT INTO ` + t + ` (` + strings.Join(insertable, ", ") + `) VALUES (` + strings.Join(placeholders, ", ") + `)`
	a.exec(q, args)
	a.log("Вставка одной записи в " + t)
}

func (a *App) insertMany(t string) {
	insertable := columns[t][1:]
	n := readInt("Сколько записей добавить? ")
	if n <= 0 {
		return
	}

	var args []interface{}
	var rows []string
	for rec := 1; rec <= n; rec++ {
		fmt.Printf("\n--- Запись %d из %d ---\n", rec, n)
		var row []string
		for _, col := range insertable {
			val := strings.TrimSpace(read(fmt.Sprintf("%s: ", col)))
			if val == "" && col != "description" && col != "country" && col != "founded_year" && col != "model" && col != "warehouse_location" {
				fmt.Printf("Поле %s обязательно\n", col)
				rec--
				continue
			}
			args = append(args, val)
			row = append(row, "$"+strconv.Itoa(len(args)))
		}
		rows = append(rows, "("+strings.Join(row, ", ")+")")
	}
	if len(rows) == 0 {
		return
	}
	q := `INSERT INTO ` + t + ` (` + strings.Join(insertable, ", ") + `) VALUES ` + strings.Join(rows, ", ")
	a.exec(q, args)
	a.log(fmt.Sprintf("Множественная вставка %d записей в %s", len(rows), t))
}

// === 5. ДОБАВЛЕНИЕ В СВЯЗАННЫЕ ТАБЛИЦЫ ===
func (a *App) insertRelated() {
	mode := a.selectMode()
	if mode == "" {
		return
	}

	if mode == "Одна" {
		a.relatedOne()
	} else {
		n := readInt("Сколько наборов добавить? ")
		if n <= 0 {
			return
		}
		success := 0
		for i := 1; i <= n; i++ {
			fmt.Printf("\n=== НАБОР %d из %d ===\n", i, n)
			if a.relatedOne() {
				success++
			}
		}
		fmt.Printf("\nУспешно добавлено наборов: %d из %d\n", success, n)
		a.log(fmt.Sprintf("Связанные вставки: %d успешных", success))
	}
}

func (a *App) relatedOne() bool {
	startTable := selectOpt("Выберите таблицу для добавления записи:", []string{"categories", "manufacturers", "components"}, true)
	if startTable == "" {
		return false
	}

	ctx := context.Background()
	tx, err := a.conn.Begin(ctx)
	if err != nil {
		a.userErr("Не удалось начать операцию")
		return false
	}
	defer tx.Rollback(ctx)

	switch startTable {
	case "categories":
		// Добавление в categories
		name := strings.TrimSpace(read("name: "))
		if name == "" {
			a.userErr("Некорректные данные в поле name")
			return false
		}
		description := strings.TrimSpace(read("description: "))

		var categoryID int
		err = tx.QueryRow(ctx, `INSERT INTO categories (name, description) VALUES ($1, $2) RETURNING id`, name, description).Scan(&categoryID)
		if err != nil {
			a.userErr("Ошибка при добавлении в categories")
			return false
		}

		// Добавление в components
		fmt.Println("Введите данные для заполнения строки в components")
		_, ok := a.addComponents(tx, ctx, categoryID, 0) // 0 для manufacturerID, будет запрошен
		if !ok {
			return false
		}

	case "manufacturers":
		// Добавление в manufacturers
		name := strings.TrimSpace(read("name: "))
		if name == "" {
			a.userErr("Некорректные данные в поле name")
			return false
		}
		country := strings.TrimSpace(read("country: "))
		yearStr := strings.TrimSpace(read("founded_year: "))
		year, err := strconv.Atoi(yearStr)
		if err != nil || year <= 1900 || year > time.Now().Year() {
			a.userErr("Некорректные данные в поле founded_year")
			return false
		}

		var manufacturerID int
		err = tx.QueryRow(ctx, `INSERT INTO manufacturers (name, country, founded_year) VALUES ($1, $2, $3) RETURNING id`, name, country, year).Scan(&manufacturerID)
		if err != nil {
			a.userErr("Ошибка при добавлении в manufacturers")
			return false
		}

		// Добавление в components
		fmt.Println("Введите данные для заполнения строки в components")
		_, ok := a.addComponents(tx, ctx, 0, manufacturerID) // 0 для categoryID, будет запрошен
		if !ok {
			return false
		}

	case "components":
		// Добавление в components
		fmt.Println("Введите данные для заполнения строки в components")
		componentID, ok := a.addComponents(tx, ctx, 0, 0) // Оба ID запросятся с проверкой
		if !ok {
			return false
		}

		// Добавление в stock
		fmt.Println("Введите данные для заполнения строки в stock")
		if !a.addStock(tx, ctx, componentID) {
			return false
		}
	}

	if err := tx.Commit(ctx); err != nil {
		a.userErr("Не удалось завершить операцию")
		return false
	}

	fmt.Println("Данные внесены в связанные таблицы")
	a.log(fmt.Sprintf("Связанная вставка в %s и связанные таблицы", startTable))
	return true
}

func (a *App) addComponents(tx pgx.Tx, ctx context.Context, knownCategoryID, knownManufacturerID int) (int, bool) {
	name := strings.TrimSpace(read("name: "))
	if name == "" {
		a.userErr("Некорректные данные в поле name")
		return 0, false
	}

	var categoryID int
	if knownCategoryID > 0 {
		categoryID = knownCategoryID
	} else {
		catIDStr := strings.TrimSpace(read("category_id: "))
		var err error
		categoryID, err = strconv.Atoi(catIDStr)
		if err != nil || categoryID <= 0 {
			a.userErr("Некорректные данные в поле category_id")
			return 0, false
		}
		exists, err := a.checkExists(tx, ctx, "categories", categoryID)
		if err != nil || !exists {
			a.userErr("Некорректный category_id")
			return 0, false
		}
	}

	var manufacturerID int
	if knownManufacturerID > 0 {
		manufacturerID = knownManufacturerID
	} else {
		manIDStr := strings.TrimSpace(read("manufacturer_id: "))
		var err error
		manufacturerID, err = strconv.Atoi(manIDStr)
		if err != nil || manufacturerID <= 0 {
			a.userErr("Некорректные данные в поле manufacturer_id")
			return 0, false
		}
		exists, err := a.checkExists(tx, ctx, "manufacturers", manufacturerID)
		if err != nil || !exists {
			a.userErr("Некорректный manufacturer_id")
			return 0, false
		}
	}

	model := strings.TrimSpace(read("model: "))
	priceStr := strings.TrimSpace(read("price: "))
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price < 0 {
		a.userErr("Некорректные данные в поле price")
		return 0, false
	}

	var componentID int
	err = tx.QueryRow(ctx, `
		INSERT INTO components (name, category_id, manufacturer_id, model, price)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		name, categoryID, manufacturerID, model, price).Scan(&componentID)
	if err != nil {
		a.userErr("Ошибка при добавлении в components")
		return 0, false
	}

	return componentID, true
}

func (a *App) addStock(tx pgx.Tx, ctx context.Context, componentID int) bool {
	quantityStr := strings.TrimSpace(read("quantity: "))
	quantity, err := strconv.Atoi(quantityStr)
	if err != nil || quantity < 0 {
		a.userErr("Некорректные данные в поле quantity")
		return false
	}
	warehouse := strings.TrimSpace(read("warehouse_location: "))

	_, err = tx.Exec(ctx, `
		INSERT INTO stock (component_id, quantity, warehouse_location)
		VALUES ($1, $2, $3)`, componentID, quantity, warehouse)
	if err != nil {
		a.userErr("Ошибка при добавлении в stock")
		return false
	}
	return true
}

func (a *App) checkExists(tx pgx.Tx, ctx context.Context, table string, id int) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)", table), id).Scan(&exists)
	return exists, err
}

// === УТИЛИТЫ ===
func (a *App) exec(q string, args []interface{}) {
	if _, e := a.conn.Exec(context.Background(), q, args...); e != nil {
		a.userErr("Не удалось выполнить операцию")
		a.logger.Println("SQL ERROR: " + e.Error() + " | QUERY: " + q)
	} else {
		fmt.Println("Готово")
	}
}

func (a *App) query(q string, args []interface{}) {
	r, e := a.conn.Query(context.Background(), q, args...)
	if e != nil {
		a.userErr("Ошибка при запросе данных")
		a.logger.Println("QUERY ERROR: " + e.Error())
		return
	}
	defer r.Close()
	printTable(r)
}

func printTable(r pgx.Rows) {
	d := r.FieldDescriptions()
	h := make([]string, len(d))
	w := make([]int, len(d))
	for i, f := range d {
		h[i] = string(f.Name)
		if len(h[i]) > w[i] {
			w[i] = len(h[i])
		}
	}

	data := [][]string{}
	for r.Next() {
		v := make([]interface{}, len(d))
		p := make([]interface{}, len(d))
		for i := range v {
			p[i] = &v[i]
		}
		r.Scan(p...)
		row := make([]string, len(d))
		for i, val := range v {
			s := "NULL"
			if val != nil {
				s = fmt.Sprint(val)
			}
			row[i] = s
			if len(s) > w[i] {
				w[i] = len(s)
			}
		}
		data = append(data, row)
	}

	printRow(h, w)
	fmt.Println(strings.Repeat("=", sum(w)+3*len(w)+1))
	for _, row := range data {
		printRow(row, w)
	}
}

func printRow(r []string, w []int) {
	p := make([]string, len(r))
	for i, c := range r {
		p[i] = pad(c, w[i])
	}
	fmt.Printf("| %s |\n", strings.Join(p, " | "))
}

func pad(s string, width int) string {
	if len(s) < width {
		return s + strings.Repeat(" ", width-len(s))
	}
	return s
}

func sum(a []int) int {
	s := 0
	for _, v := range a {
		s += v
	}
	return s
}

func readInt(p string) int {
	for {
		if n, e := strconv.Atoi(read(p)); e == nil && n > 0 {
			return n
		}
		fmt.Println("Введите число больше 0")
	}
}

func split(s string) []string {
	p := strings.Split(s, ",")
	for i := range p {
		p[i] = strings.TrimSpace(p[i])
	}
	return p
}

func (a *App) log(m string) {
	a.logger.Println(m)
}

func (a *App) userErr(m string) {
	a.logger.Println("USER ERROR: " + m)
	fmt.Fprintln(os.Stderr, "Ошибка: "+m)
}
